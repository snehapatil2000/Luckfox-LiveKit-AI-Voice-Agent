from agent import _barge_in_command


def test_barge_in_uses_wake_word_after_speaker_echo() -> None:
    assert (
        _barge_in_command(
            "There lived a boy named Arjun. Hey Siri, what is the time?"
        )
        == "what is the time?"
    )


def test_barge_in_requires_wake_word() -> None:
    assert _barge_in_command("There lived a boy named Arjun.") is None


if __name__ == "__main__":
    test_barge_in_uses_wake_word_after_speaker_echo()
    test_barge_in_requires_wake_word()
    print("wake-word checks passed")
