import asyncio
import json
from types import SimpleNamespace
from unittest.mock import AsyncMock, Mock

import agent
from agent import Assistant, QUESTION_START_TIMEOUT, select_response_language
import pytest


def test_wake_word_timing_is_one_point_five_seconds() -> None:
    assert QUESTION_START_TIMEOUT == 1.5


def test_wake_word_arms_one_question() -> None:
    sleep_events = []
    assistant = Assistant(english_tts=None, on_sleep=sleep_events.append)
    assistant.arm_from_wake_word()
    assert assistant._activation_armed
    assert not assistant.consume_activation()
    assert assistant.mark_activation_speech_started()
    assert assistant.consume_activation()
    assert sleep_events == ["question_complete"]
    assert not assistant.consume_activation()


def test_silent_wake_word_expires() -> None:
    sleep_events = []
    assistant = Assistant(english_tts=None, on_sleep=sleep_events.append)
    assistant.arm_from_wake_word()
    assert assistant.expire_activation()
    assert not assistant._activation_armed
    assert sleep_events == ["no_speech"]


def test_started_speech_does_not_expire() -> None:
    assistant = Assistant(english_tts=None)
    assistant.arm_from_wake_word()
    assert assistant.mark_activation_speech_started()
    assert not assistant.expire_activation()
    assert assistant.consume_activation()


def test_response_language_follows_current_question() -> None:
    assert select_response_language("What is the capital of Karnataka?") == "en"
    assert select_response_language("ಕರ್ನಾಟಕದ ರಾಜಧಾನಿ ಯಾವುದು?") == "kn"
    assert select_response_language("Explain Karnataka in Kannada") == "kn"
    assert select_response_language("ಇದನ್ನು ಇಂಗ್ಲಿಷ್‌ನಲ್ಲಿ ವಿವರಿಸಿ") == "en"


@pytest.mark.asyncio
async def test_session_startup_and_silent_wake_return_to_standby(monkeypatch):
    # Run the real entrypoint and control handlers without cloud/provider calls.
    handlers = {}
    room_handlers = {}

    def register(target):
        def on(name):
            def decorate(callback):
                target[name] = callback
                return callback
            return decorate
        return on

    session = Mock()
    session.on = register(handlers)
    session.start = AsyncMock()
    session.current_speech = None
    session.input = Mock()
    participant = SimpleNamespace(publish_data=AsyncMock())
    room = SimpleNamespace(on=register(room_handlers), local_participant=participant)
    ctx = SimpleNamespace(room=room, connect=AsyncMock())
    monkeypatch.setattr(agent, "AgentSession", lambda **kwargs: session)
    monkeypatch.setattr(agent, "SARVAM_ENABLED", False)
    monkeypatch.setattr(agent.deepgram, "TTS", Mock())
    monkeypatch.setattr(agent.deepgram, "STT", Mock())
    monkeypatch.setattr(agent.openai.LLM, "with_deepseek", Mock())
    monkeypatch.setattr(agent.inference, "TurnDetector", Mock())
    monkeypatch.setattr(agent, "QUESTION_START_TIMEOUT", 0.01)

    await agent.my_agent(ctx)
    assistant = session.start.call_args.kwargs["agent"]
    messages = lambda: [json.loads(call.args[0]) for call in participant.publish_data.call_args_list]
    assert messages() == [{"type": "mic_sleep", "reason": "startup"}]
    assert not assistant._activation_armed

    # Speech alone must not bypass the local wake event.
    handlers["user_state_changed"](SimpleNamespace(new_state="speaking"))
    assert not assistant._activation_armed

    # A local wake interrupts even speech configured as acoustically
    # uninterruptible. The new question is armed only after the beep.
    session.current_speech = SimpleNamespace(interrupted=False)
    room_handlers["data_received"](SimpleNamespace(
        topic=agent.CONTROL_TOPIC, data=b'{"type":"interrupt_answer"}'
    ))
    session.interrupt.assert_called_once_with(force=True)
    assert not assistant._activation_armed
    session.input.set_audio_enabled.assert_called_with(False)

    room_handlers["data_received"](SimpleNamespace(
        topic=agent.CONTROL_TOPIC, data=b'{"type":"arm_question"}'
    ))
    assert assistant._activation_armed
    session.input.set_audio_enabled.assert_called_with(True)
    await asyncio.sleep(0.05)
    assert not assistant._activation_armed
    assert messages()[-1] == {"type": "mic_sleep", "reason": "no_speech"}
    session.input.set_audio_enabled.assert_called_with(False)


if __name__ == "__main__":
    test_wake_word_timing_is_one_point_five_seconds()
    test_wake_word_arms_one_question()
    test_silent_wake_word_expires()
    test_started_speech_does_not_expire()
    test_response_language_follows_current_question()
    print("wake-word checks passed")
