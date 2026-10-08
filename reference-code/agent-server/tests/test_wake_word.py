from agent import Assistant, BUTTON_SPEECH_TIMEOUT, select_response_language


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


def test_response_language_follows_current_question() -> None:
    assert select_response_language("What is the capital of Karnataka?") == "en"
    assert select_response_language("ಕರ್ನಾಟಕದ ರಾಜಧಾನಿ ಯಾವುದು?") == "kn"
    assert select_response_language("Explain Karnataka in Kannada") == "kn"
    assert select_response_language("ಇದನ್ನು ಇಂಗ್ಲಿಷ್‌ನಲ್ಲಿ ವಿವರಿಸಿ") == "en"


if __name__ == "__main__":
    test_button_timing_is_three_seconds()
    test_button_arms_one_question()
    test_silent_button_press_expires()
    test_started_speech_does_not_expire()
    test_response_language_follows_current_question()
    print("button-only checks passed")
