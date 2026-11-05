import os
import unittest

from turn_aggregator import capture_turn, default_continuation_timeout, join_turn


class WaitTimeoutError(Exception):
    pass


class _Recognizer:
    def __init__(self, items):
        self.items = list(items)
        self.calls = []

    def listen(self, source, timeout, phrase_time_limit):
        self.calls.append((timeout, phrase_time_limit))
        if not self.items:
            raise WaitTimeoutError()
        item = self.items.pop(0)
        if isinstance(item, Exception):
            raise item
        return item


class TurnAggregatorTests(unittest.TestCase):
    def test_join_turn_preserves_segment_order(self):
        self.assertEqual(
            join_turn(["uno", "dos", "", "tres"]),
            "uno dos tres",
        )


    def test_default_continuation_timeout_is_channel_configurable(self):
        previous = os.environ.get("AHMELIA_VOICE_CONTINUATION_TIMEOUT")
        try:
            os.environ["AHMELIA_VOICE_CONTINUATION_TIMEOUT"] = "8.0"
            self.assertEqual(default_continuation_timeout(), 8.0)
            os.environ["AHMELIA_VOICE_CONTINUATION_TIMEOUT"] = "99"
            self.assertEqual(default_continuation_timeout(), 15.0)
        finally:
            if previous is None:
                os.environ.pop("AHMELIA_VOICE_CONTINUATION_TIMEOUT", None)
            else:
                os.environ["AHMELIA_VOICE_CONTINUATION_TIMEOUT"] = previous

    def test_capture_turn_keeps_adjacent_segments_until_real_silence(self):
        recognizer = _Recognizer(["segmento 1", "segmento 2", WaitTimeoutError()])
        texts = capture_turn(
            recognizer,
            object(),
            lambda audio: audio,
            first_timeout=10,
            phrase_time_limit=10,
            continuation_timeout=5.0,
            max_segments=8,
            max_duration=20,
        )
        self.assertEqual(texts, ["segmento 1", "segmento 2"])
        self.assertEqual(recognizer.calls[0], (10, 10))
        self.assertEqual(recognizer.calls[1][0], 5.0)

    def test_capture_turn_ignores_empty_first_transcription_without_index_error(self):
        recognizer = _Recognizer(["", WaitTimeoutError()])
        texts = capture_turn(
            recognizer,
            object(),
            lambda audio: audio,
            first_timeout=10,
            phrase_time_limit=10,
            continuation_timeout=5.0,
            max_segments=8,
            max_duration=20,
        )
        self.assertEqual(texts, [])


if __name__ == "__main__":
    unittest.main()
