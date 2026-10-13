/* Native 16 kHz PCM keyword spotting: emit as soon as a keyword is found,
 * without waiting for the CLI live endpointer to close a speech segment.
 * Uses the already installed PocketSphinx acoustic model and dictionary. */
#include <pocketsphinx.h>
#include <pocketsphinx/endpointer.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

static void emit_result(const char *text)
{
    fputs("{\"t\":\"", stdout);
    for (; *text; ++text) {
        if (*text == '"' || *text == '\\')
            fputc('\\', stdout);
        if ((unsigned char)*text >= 32)
            fputc(*text, stdout);
    }
    fputs("\"}\n", stdout);
    fflush(stdout);
}

static int emit_keyword(ps_decoder_t *decoder, size_t samples, clock_t started)
{
    const char *hypothesis = ps_get_hyp(decoder, NULL);
    if (!hypothesis || !*hypothesis)
        return 0;
    fprintf(stderr, "WAKE MATCH: audio=%.2fs decoder_cpu=%.2fs\n",
            samples / 16000.0, (double)(clock() - started) / CLOCKS_PER_SEC);
    emit_result(hypothesis);
    return 1;
}

static ps_endpointer_t *wake_endpointer(void)
{
    /* Loose VAD and a short onset window keep quiet speech eligible while
     * avoiding full Gaussian scoring on every background-noise frame. */
    return ps_endpointer_init(0.12, 0.6, PS_VAD_LOOSE, 16000, 0.02);
}

int main(int argc, char **argv)
{
    ps_config_t *config = ps_config_init(NULL);
    ps_decoder_t *decoder;
    ps_endpointer_t *endpointer = NULL;
    int16_t frame[320];
    size_t count;
    int result = 0;
    int utterance_open = 0;
    int keyword_emitted = 0;
    size_t processed_samples = 0;
    clock_t decode_start;

    if (!config)
        return 1;
    ps_config_set_str(config, "loglevel", "ERROR");
    ps_config_set_int(config, "samprate", 16000);
    ps_config_set_int(config, "kws_delay", 2);
    /* Reuse Gaussian candidates on alternate feature frames. Keep all four
     * scoring candidates to preserve the acoustic model's discrimination. */
    ps_config_set_int(config, "ds", 2);

    /* Accept the existing client command, including the trailing `live -`. */
    for (int i = 1; i < argc; i += 2) {
        if (strcmp(argv[i], "live") == 0)
            break;
        if (i + 1 >= argc) {
            fprintf(stderr, "wake detector: option requires a value\n");
            ps_config_free(config);
            return 2;
        }
        if (strcmp(argv[i], "-hmm") == 0 ||
            strcmp(argv[i], "-dict") == 0 ||
            strcmp(argv[i], "-keyphrase") == 0)
            ps_config_set_str(config, argv[i] + 1, argv[i + 1]);
        else if (strcmp(argv[i], "-samprate") == 0)
            ps_config_set_int(config, "samprate", strtol(argv[i + 1], NULL, 10));
        else if (strcmp(argv[i], "-kws_threshold") == 0)
            ps_config_set_float(config, argv[i] + 1, atof(argv[i + 1]));
        else {
            fprintf(stderr, "wake detector: unknown option %s\n", argv[i]);
            ps_config_free(config);
            return 2;
        }
    }
    if (ps_config_int(config, "samprate") != 16000) {
        fprintf(stderr, "wake detector requires 16000 Hz PCM\n");
        ps_config_free(config);
        return 2;
    }
    decoder = ps_init(config);
    if (!decoder) {
        ps_config_free(config);
        return 1;
    }
    setvbuf(stdin, NULL, _IONBF, 0);
    endpointer = wake_endpointer();
    if (!endpointer || ps_endpointer_frame_size(endpointer) != 320 || ps_start_stream(decoder) < 0) {
        result = 1;
        goto cleanup;
    }
    fputs("{\"ready\":true}\n", stdout);
    fflush(stdout);
    decode_start = clock();
    while ((count = fread(frame, sizeof(frame[0]), 320, stdin)) != 0) {
        const int16_t *speech;
        size_t speech_samples = count;
        processed_samples += count;
        if (count == 320)
            speech = ps_endpointer_process(endpointer, frame);
        else
            speech = ps_endpointer_end_stream(endpointer, frame, count, &speech_samples);
        if (!speech)
            continue;
        if (!utterance_open) {
            if (ps_start_utt(decoder) < 0) {
                result = 1;
                goto cleanup;
            }
            utterance_open = 1;
            keyword_emitted = 0;
        }
        if (!keyword_emitted && ps_process_raw(decoder, speech, speech_samples, 0, 0) < 0) {
            fprintf(stderr, "wake detector: PCM processing failed\n");
            result = 1;
            break;
        }
        if (!keyword_emitted && emit_keyword(decoder, processed_samples, decode_start))
            keyword_emitted = 1;
        /* One speech segment produces at most one wake event. In particular,
         * do not start a fresh search on the trailing sound of the same word. */
        if (!ps_endpointer_in_speech(endpointer)) {
            ps_end_utt(decoder);
            utterance_open = 0;
            if (!keyword_emitted)
                emit_keyword(decoder, processed_samples, decode_start);
            keyword_emitted = 0;
        }
    }
    if (utterance_open)
        ps_end_utt(decoder);
cleanup:
    if (endpointer)
        ps_endpointer_free(endpointer);
    ps_free(decoder);
    ps_config_free(config);
    return result;
}
