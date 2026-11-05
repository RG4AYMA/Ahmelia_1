package inputprotocol

import (
	"strings"
	"time"
)

// CoalesceTextLines converts line-oriented stdin into logical text turns.
// Newlines are content; they are not, by themselves, turn boundaries.
// A short idle window separates a pasted/multi-line block from the next
// interactive command. Semantic objective assembly remains the fallback for
// genuinely multi-turn text conversations.
func CoalesceTextLines(lines <-chan string, idle time.Duration, emit func(string)) {
	if idle <= 0 {
		idle = 150 * time.Millisecond
	}

	var pending []string
	var timer *time.Timer
	var timerC <-chan time.Time

	flush := func() {
		if len(pending) == 0 {
			return
		}
		text := strings.TrimSpace(strings.Join(pending, "\n"))
		pending = nil
		if text != "" {
			emit(text)
		}
	}
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerC = nil
	}
	armTimer := func() {
		if timer == nil {
			timer = time.NewTimer(idle)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idle)
		}
		timerC = timer.C
	}

	for lines != nil || timerC != nil {
		select {
		case line, ok := <-lines:
			if !ok {
				lines = nil
				stopTimer()
				flush()
				continue
			}
			pending = append(pending, line)
			armTimer()
		case <-timerC:
			timerC = nil
			flush()
		}
	}
}
