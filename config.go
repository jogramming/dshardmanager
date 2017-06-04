package dshardmanager

import ()

type GuildsProvider interface {
	ConnectedGuilds() []string
}

type optFunc func(m *Manager)

func OptguildsProvider(g GuildsProvider) optFunc {
	return func(m *Manager) {
		m.GuildsProvider = g
	}
}

func OptLogChannel(channelID string) optFunc {
	return func(m *Manager) {
		m.LogChannel = channelID
	}
}

func OptLogEventsToDiscord(logConnectionEvents, enableUpdatedStatusMessage bool) optFunc {
	return func(m *Manager) {
		m.EnableUpdatedStatusMessageToDiscord = enableUpdatedStatusMessage
		m.LogConnectionEventsToDiscord = logConnectionEvents
	}
}

func OptOnEvent(onEvt func(evt *Event)) optFunc {
	return func(m *Manager) {
		m.OnEvent = onEvt
	}
}

func OptSessionFunc(sf SessionFunc) optFunc {
	return func(m *Manager) {
		m.SessionFunc = sf
	}
}
