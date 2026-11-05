# -*- coding: utf-8 -*-
"""Channel-level speech turn aggregation.

Speech recognizers naturally segment one human utterance at pauses. This module
reassembles adjacent speech segments into one conversational turn until a real
silence window is observed. It contains no Ahmelia/domain semantics.
"""
import os
import time


def default_phrase_time_limit():
    """Maximum duration of one recognition segment before turn aggregation continues.

    Long objectives are still one human intervention. The recognizer must not
    manufacture turn boundaries merely because a phrase exceeded a short
    recognition window.
    """
    try:
        value = float(os.getenv("AHMELIA_VOICE_PHRASE_TIME_LIMIT", "45.0"))
    except (TypeError, ValueError):
        value = 45.0
    return min(max(value, 10.0), 60.0)


def default_continuation_timeout():
    """Return the channel silence window used to close a spoken turn.

    The policy is channel-level and domain-neutral. Hosts can tune it without
    modifying the voice adapter when a user naturally pauses longer between
    sections of a single spoken objective.
    """
    try:
        value = float(os.getenv("AHMELIA_VOICE_CONTINUATION_TIMEOUT", "12.0"))
    except (TypeError, ValueError):
        value = 5.0
    return min(max(value, 1.0), 15.0)


def _needs_extended_continuation(text):
    """Detect a linguistically open segment without knowing the domain.

    A colon, trailing conjunction, or delegation preamble means a human is
    plausibly continuing a single thought after a pause. This only affects the
    channel's waiting window; it never classifies the task or grants authority.
    """
    normalized = " ".join((text or "").strip().lower().split())
    if not normalized:
        return False
    if normalized.endswith(":"):
        return True
    if normalized.endswith((",", ";", "-", "y", "o", "que", "si", "porque", "para")):
        return True
    prefixes = (
        "quiero que ", "necesito que ", "me gustaria que ", "me gustaría que ",
        "i want you to ", "i need you to ",
    )
    return normalized.startswith(prefixes)


def capture_turn(recognizer, source, transcribe, *, first_timeout=10,
                 phrase_time_limit=None, continuation_timeout=None,
                 max_segments=32, max_duration=120):
    if phrase_time_limit is None:
        phrase_time_limit = default_phrase_time_limit()
    if continuation_timeout is None:
        continuation_timeout = default_continuation_timeout()
    started = time.monotonic()
    texts = []

    try:
        audio = recognizer.listen(source, timeout=first_timeout,
                                  phrase_time_limit=phrase_time_limit)
    except Exception as exc:
        # speech_recognition exposes WaitTimeoutError; keep this helper
        # dependency-light by letting the caller handle the empty turn.
        if exc.__class__.__name__ == "WaitTimeoutError":
            return []
        raise

    text = transcribe(audio).strip()
    if text:
        texts.append(text)
    else:
        # A recognizer can legally return an empty transcription. Do not enter
        # the continuation loop with an empty segment: texts[-1] would be an
        # invalid indexing operation and would kill the voice channel.
        return []

    while len(texts) < max_segments and time.monotonic() - started < max_duration:
        wait_timeout = min(15.0, max(continuation_timeout, 15.0 if _needs_extended_continuation(texts[-1]) else continuation_timeout))
        try:
            audio = recognizer.listen(source, timeout=wait_timeout,
                                      phrase_time_limit=phrase_time_limit)
        except Exception as exc:
            if exc.__class__.__name__ == "WaitTimeoutError":
                break
            raise
        text = transcribe(audio).strip()
        if text:
            texts.append(text)

    return texts


def join_turn(texts):
    return " ".join(t.strip() for t in texts if t and t.strip()).strip()
