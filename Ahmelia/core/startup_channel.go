package core

import "ahmelia/interaction"

// SetStartupChannel declara el canal real por el que Ahmelia puede hablar antes
// de que llegue el primer turno del usuario. El árbitro exige conocer el canal
// para dejar hablar a la iniciativa sin turno del usuario; hasta ahora solo se
// llenaba al recibir una entrada. No pisa un canal ya observado y ignora
// canales vacíos o desconocidos.
func (e *Engine) SetStartupChannel(channel interaction.InputChannel) {
	if e == nil || channel == "" || channel == interaction.ChannelUnknown {
		return
	}
	e.autonomyRuntimeMu.Lock()
	defer e.autonomyRuntimeMu.Unlock()
	if e.lastInputChannel == "" || e.lastInputChannel == interaction.ChannelUnknown {
		e.lastInputChannel = channel
	}
}
