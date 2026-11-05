package interaction

import "testing"

func TestAnalyzePreservesInputChannel(t *testing.T) {
	voice := Analyze("crea una función", Context{Channel: ChannelVoice})
	text := Analyze("crea una función", Context{Channel: ChannelText})
	if voice.Channel != ChannelVoice || text.Channel != ChannelText {
		t.Fatalf("channel provenance lost: voice=%q text=%q", voice.Channel, text.Channel)
	}
}
