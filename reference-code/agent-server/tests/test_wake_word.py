from agent import Assistant, BUTTON_SPEECH_TIMEOUT


def test_button_timing_is_three_seconds() -> None:
    assert BUTTON_SPEECH_TIMEOUT == 3.0


def test_button_arms_one_question() -> None:
    sleep_events = []
    assistant = Assistant(english_tts=None, on_sleep=lambda: sleep_events.append(True))
    assistant.arm_from_button()
    assert assistant._button_armed
    assert not assistant.consume_button_arm()
    assert assistant.mark_button_speech_started()
    assert assistant.consume_button_arm()
    assert sleep_events == [True]
    assert not assistant.consume_button_arm()


def test_silent_button_press_expires() -> None:
    sleep_events = []
    assistant = Assistant(english_tts=None, on_sleep=lambda: sleep_events.append(True))
    assistant.arm_from_button()
    assert assistant.expire_button_arm()
    assert not assistant._button_armed
    assert sleep_events == [True]


def test_started_speech_does_not_expire() -> None:
    assistant = Assistant(english_tts=None)
    assistant.arm_from_button()
    assert assistant.mark_button_speech_started()
    assert not assistant.expire_button_arm()
    assert assistant.consume_button_arm()


if __name__ == "__main__":
    test_button_timing_is_three_seconds()
    test_button_arms_one_question()
    test_silent_button_press_expires()
    test_started_speech_does_not_expire()
    print("button-only checks passed")
