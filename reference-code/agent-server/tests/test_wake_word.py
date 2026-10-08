from agent import Assistant


def test_button_arms_one_question() -> None:
    assistant = Assistant(english_tts=None)
    assistant.arm_from_button()
    assert assistant._button_armed
    assert assistant.consume_button_arm()
    assert not assistant.consume_button_arm()


if __name__ == "__main__":
    test_button_arms_one_question()
    print("button-only checks passed")
