// Native microWakeWord streaming adapter. One 16 kHz, mono PCM input on stdin.
#include <algorithm>
#include <cmath>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>
extern "C" {
#include "tensorflow/lite/experimental/microfrontend/lib/frontend.h"
#include "tensorflow/lite/experimental/microfrontend/lib/frontend_util.h"
struct TfLiteModel;
struct TfLiteInterpreter;
struct TfLiteInterpreterOptions;
struct TfLiteTensor;
struct TfLiteQuantizationParams { float scale; int32_t zero_point; };
const char *TfLiteVersion();
TfLiteModel *TfLiteModelCreateFromFile(const char *);
void TfLiteModelDelete(TfLiteModel *);
TfLiteInterpreterOptions *TfLiteInterpreterOptionsCreate();
void TfLiteInterpreterOptionsSetNumThreads(TfLiteInterpreterOptions *, int32_t);
void TfLiteInterpreterOptionsDelete(TfLiteInterpreterOptions *);
TfLiteInterpreter *TfLiteInterpreterCreate(const TfLiteModel *, const TfLiteInterpreterOptions *);
void TfLiteInterpreterDelete(TfLiteInterpreter *);
int TfLiteInterpreterAllocateTensors(TfLiteInterpreter *);
int TfLiteInterpreterInvoke(TfLiteInterpreter *);
TfLiteTensor *TfLiteInterpreterGetInputTensor(TfLiteInterpreter *, int32_t);
const TfLiteTensor *TfLiteInterpreterGetOutputTensor(const TfLiteInterpreter *, int32_t);
int TfLiteTensorType(const TfLiteTensor *);
int TfLiteTensorDim(const TfLiteTensor *, int32_t);
size_t TfLiteTensorByteSize(const TfLiteTensor *);
TfLiteQuantizationParams TfLiteTensorQuantizationParams(const TfLiteTensor *);
int TfLiteTensorCopyFromBuffer(TfLiteTensor *, const void *, size_t);
int TfLiteTensorCopyToBuffer(const TfLiteTensor *, void *, size_t);
}

int main(int argc, char **argv) {
    if (argc != 5) {
        std::fprintf(stderr, "Usage: microWakeWord MODEL.tflite WORD CUTOFF WINDOW < PCM16\n");
        return 2;
    }
    char *end;
    const float cutoff = std::strtof(argv[3], &end);
    if (end == argv[3] || *end || !std::isfinite(cutoff) || cutoff <= 0 || cutoff > 1) return 2;
    const long window = std::strtol(argv[4], &end, 10);
    if (end == argv[4] || *end || window < 1 || window > 200) return 2;
    for (const char *p = argv[2]; *p; ++p)
        if (!((*p >= 'a' && *p <= 'z') || (*p >= 'A' && *p <= 'Z') || *p == '_' || *p == '-')) return 2;
    if (!*argv[2] || std::strlen(argv[2]) > 64) return 2;

    TfLiteModel *model = TfLiteModelCreateFromFile(argv[1]);
    if (!model) { std::fputs("microWakeWord: cannot load model\n", stderr); return 1; }
    TfLiteInterpreterOptions *options = TfLiteInterpreterOptionsCreate();
    TfLiteInterpreterOptionsSetNumThreads(options, 1);
    TfLiteInterpreter *interpreter = TfLiteInterpreterCreate(model, options);
    TfLiteInterpreterOptionsDelete(options);
    if (!interpreter || TfLiteInterpreterAllocateTensors(interpreter) != 0) {
        std::fputs("microWakeWord: cannot initialize interpreter\n", stderr);
        if (interpreter) TfLiteInterpreterDelete(interpreter);
        TfLiteModelDelete(model);
        return 1;
    }
    TfLiteTensor *input = TfLiteInterpreterGetInputTensor(interpreter, 0);
    const TfLiteTensor *output = TfLiteInterpreterGetOutputTensor(interpreter, 0);
    const int stride = input ? TfLiteTensorDim(input, 1) : 0;
    const int input_type = input ? TfLiteTensorType(input) : 0;
    const int output_type = output ? TfLiteTensorType(output) : 0;
    if (!input || !output || stride < 1 || stride > 100 || TfLiteTensorDim(input, 2) != 40 ||
        (input_type != 3 && input_type != 9) || (output_type != 3 && output_type != 9) ||
        TfLiteTensorByteSize(input) != static_cast<size_t>(stride * 40) || TfLiteTensorByteSize(output) != 1) {
        std::fputs("microWakeWord: unsupported model tensor layout\n", stderr);
        TfLiteInterpreterDelete(interpreter); TfLiteModelDelete(model); return 1;
    }
    const auto in_q = TfLiteTensorQuantizationParams(input);
    const auto out_q = TfLiteTensorQuantizationParams(output);
    if (!(in_q.scale > 0) || !(out_q.scale > 0)) {
        TfLiteInterpreterDelete(interpreter); TfLiteModelDelete(model); return 1;
    }
    FrontendConfig config{};
    FrontendFillConfigWithDefaults(&config);
    config.window.size_ms = 30; config.window.step_size_ms = 10;
    config.filterbank.num_channels = 40;
    config.filterbank.lower_band_limit = 125.0f; config.filterbank.upper_band_limit = 7500.0f;
    config.noise_reduction.smoothing_bits = 10;
    config.noise_reduction.even_smoothing = 0.025f; config.noise_reduction.odd_smoothing = 0.06f;
    config.noise_reduction.min_signal_remaining = 0.05f;
    config.pcan_gain_control.enable_pcan = 1; config.pcan_gain_control.strength = 0.95f;
    config.pcan_gain_control.offset = 80.0f; config.pcan_gain_control.gain_bits = 21;
    config.log_scale.enable_log = 1; config.log_scale.scale_shift = 6;
    FrontendState frontend{};
    if (!FrontendPopulateState(&config, &frontend, 16000)) {
        TfLiteInterpreterDelete(interpreter); TfLiteModelDelete(model); return 1;
    }
    std::vector<uint8_t> features(stride * 40);
    std::vector<float> history(window, 0);
    int feature_count = 0, history_count = 0, history_index = 0, result = 0;
    float history_sum = 0;
    bool triggered = false;
    size_t samples_total = 0;
    std::fprintf(stderr, "microWakeWord: TFLite %s, stride=%d, cutoff=%.3f, window=%ld\n", TfLiteVersion(), stride, cutoff, window);
    std::puts("{\"ready\":true}"); std::fflush(stdout);
    int16_t pcm[160];
    size_t count;
    while ((count = std::fread(pcm, sizeof(pcm[0]), 160, stdin)) > 0 && result == 0) {
        samples_total += count;
        size_t offset = 0;
        while (offset < count) {
            size_t consumed = 0;
            auto value = FrontendProcessSamples(&frontend, pcm + offset, count - offset, &consumed);
            if (!consumed || consumed > count - offset) { result = 1; break; }
            offset += consumed;
            if (value.size == 0) continue;
            if (value.size != 40) { result = 1; break; }
            for (size_t i = 0; i < 40; ++i) {
                double q = std::nearbyint((value.values[i] * 0.0390625) / in_q.scale + in_q.zero_point);
                q = std::max(input_type == 9 ? -128.0 : 0.0, std::min(input_type == 9 ? 127.0 : 255.0, q));
                features[feature_count * 40 + i] = static_cast<uint8_t>(static_cast<int>(q));
            }
            if (++feature_count < stride) continue;
            feature_count = 0;
            uint8_t probability;
            if (TfLiteTensorCopyFromBuffer(input, features.data(), features.size()) != 0 ||
                TfLiteInterpreterInvoke(interpreter) != 0 || TfLiteTensorCopyToBuffer(output, &probability, 1) != 0) {
                result = 1; break;
            }
            int quantized = output_type == 9 ? static_cast<int8_t>(probability) : probability;
            float score = (quantized - out_q.zero_point) * out_q.scale;
            history_sum += score - history[history_index]; history[history_index] = score;
            history_index = (history_index + 1) % window;
            if (history_count < window) ++history_count;
            bool detected = history_count == window && history_sum / window > cutoff;
            if (detected && !triggered) {
                std::printf("{\"t\":\"%s\",\"audio_seconds\":%.3f,\"probability\":%.4f}\n", argv[2], samples_total / 16000.0, history_sum / window);
                std::fflush(stdout);
            }
            triggered = detected;
        }
    }
    if (std::ferror(stdin)) result = 1;
    FrontendFreeStateContents(&frontend);
    TfLiteInterpreterDelete(interpreter); TfLiteModelDelete(model);
    return result;
}
