package dshardmanager

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SessionFunc func(token string) (*discordgo.Session, error)

type Manager struct {
	sync.RWMutex

	// All the shard sessions
	Sessions      []*discordgo.Session
	eventHandlers []interface{}

	// Channel to log connection events and warnings in and keep the
	// Updated status message in
	LogChannel string

	// Enable to log connection events to discord
	LogConnectionEventsToDiscord bool

	// Enables an updated status message in the channel
	EnableUpdatedStatusMessageToDiscord bool

	// GuildsProvider is used for stastics, it is required if you want the updating status message
	GuildsProvider GuildsProvider

	// Called on events, by default this is set to a function that logs it to log.Printf
	// You can override this if you want another behaviour, or just set it to nil for nothing.
	OnEvent     func(e *Event)
	SessionFunc SessionFunc

	nextStatusUpdate     time.Time
	statusUpdaterStarted bool

	numShards int
	token     string

	bareSession *discordgo.Session
}

// New creates a new shard manager, after you have created this you call Manager.Start
// To start connecting
//
// options is a varadic argument of func(*Manager),
// this is what this package uses for setup configuration
// The appropiate functions are prefixed with Opt
//
// Example:
// dshardmanager.New("Bot asd", OptLogChannel(someChannel), OptLogEventsToDiscord(true, true))
func New(token string, options ...func(m *Manager)) *Manager {
	// Setup defaults
	manager := &Manager{
		token: token,
	}
	manager.OnEvent = manager.LogConnectionEventStd
	manager.SessionFunc = manager.StdSessionFunc
	manager.GuildsProvider = &StdGuildsProvider{Manager: manager}

	manager.bareSession, _ = discordgo.New(token)

	for _, v := range options {
		v(manager)
	}

	return manager
}

func (m *Manager) GetRecommendedCount() (int, error) {
	resp, err := m.bareSession.GatewayBot()
	if err != nil {
		return 0, errors.WithMessage(err, "GetRecommendedCount()")
	}

	m.numShards = resp.Shards
	if m.numShards < 1 {
		m.numShards = 1
	}

	return m.numShards, nil
}

// Adds an event handler to all shards
// All event handlers will be added to new sessions automatically.
func (m *Manager) AddHandler(handler interface{}) {
	m.Lock()
	defer m.Unlock()
	m.eventHandlers = append(m.eventHandlers, handler)

	if len(m.Sessions) > 0 {
		for _, v := range m.Sessions {
			v.AddHandler(handler)
		}
	}
}

func (m *Manager) Start() error {

	m.Lock()
	defer m.Unlock()
	if m.numShards < 0 {
		_, err := m.GetRecommendedCount()
		if err != nil {
			return errors.WithMessage(err, "Start")
		}
	}

	m.Sessions = make([]*discordgo.Session, m.numShards)
	for i := 0; i < m.numShards; i++ {
		err := m.startSession(i)
		if err != nil {
			return errors.WithMessage(err, fmt.Sprintf("Failed starting shard %d", i))
		}
	}

	if !m.statusUpdaterStarted {
		m.statusUpdaterStarted = true
		go m.statusRoutine()
	}
	m.nextStatusUpdate = time.Now()

	return nil
}

func (m *Manager) startSession(shard int) error {
	session, err := m.SessionFunc(m.token)
	if err != nil {
		return errors.WithMessage(err, "startSession.SessionFunc")
	}

	session.ShardCount = m.numShards
	session.ShardID = shard

	session.AddHandler(m.OnDiscordConnected)
	session.AddHandler(m.OnDiscordDisconnected)
	session.AddHandler(m.OnDiscordReady)
	session.AddHandler(m.OnDiscordResumed)

	// Add the user event handlers retroactively
	for _, v := range m.eventHandlers {
		session.AddHandler(v)
	}

	err = session.Open()
	if err != nil {
		return errors.Wrap(err, "startSession.Open")
	}
	m.handleEvent(EventOpen, shard, "")

	m.Sessions[shard] = session
	return nil
}

// Same as SessionForGuild but accepts the guildID as a string for convenience
func (m *Manager) SessionForGuildS(guildID string) *discordgo.Session {
	// Question is, should we really ignore this error?
	// In reality, the guildID should never be invalid but...
	parsed, _ := strconv.ParseInt(guildID, 10, 64)
	return m.SessionForGuild(parsed)
}

// Returns the session for the specified guild
func (m *Manager) SessionForGuild(guildID int64) *discordgo.Session {
	// (guild_id >> 22) % num_shards == shard_id
	// That formula is taken from the sharding issue on the api docs repository on github
	m.RLock()
	defer m.RUnlock()
	shardID := (guildID >> 22) % int64(m.numShards)
	return m.Sessions[shardID]
}

func (m *Manager) Session(shardID int) *discordgo.Session {
	m.RLock()
	defer m.RUnlock()
	return m.Sessions[shardID]
}

func (m *Manager) LogConnectionEventStd(e *Event) {
	log.Printf("[Shard Manager] %s", e.String())
}

func (m *Manager) handleError(err error, shard int, msg string) bool {
	if err == nil {
		return false
	}

	m.handleEvent(EventError, shard, msg+": "+err.Error())
	return true
}

func (m *Manager) handleEvent(typ EventType, shard int, msg string) {
	if m.OnEvent == nil {
		return
	}

	evt := &Event{
		Type:      typ,
		Shard:     shard,
		NumShards: m.numShards,
		Msg:       msg,
		Time:      time.Now(),
	}

	go m.OnEvent(evt)

	if m.LogChannel != "" && m.LogConnectionEventsToDiscord {
		go m.logEventToDiscord(evt)
	}

	go func() {
		m.Lock()
		m.nextStatusUpdate = time.Now().Add(time.Second * 2)
		m.Unlock()
	}()
}

func (m *Manager) StdSessionFunc(token string) (*discordgo.Session, error) {
	s, err := discordgo.New(token)
	if err != nil {
		return nil, errors.WithMessage(err, "StdSessionFunc")
	}
	return s, nil
}

func (m *Manager) logEventToDiscord(evt *Event) {
	if evt.Type == EventError {
		return
	}

	str := evt.String()
	_, err := m.bareSession.ChannelMessageSend(m.LogChannel, str)
	m.handleError(err, evt.Shard, "Failed sending event to discord")
}

func (m *Manager) statusRoutine() {
	if m.LogChannel == "" || !m.EnableUpdatedStatusMessageToDiscord || m.GuildsProvider == nil {
		return
	}

	mID := ""
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-ticker.C:
			m.RLock()
			after := time.Now().After(m.nextStatusUpdate)
			m.RUnlock()
			if after {
				m.Lock()
				m.nextStatusUpdate = time.Now().Add(time.Minute)
				m.Unlock()

				nID, err := m.updateStatusMessage(mID)
				if !m.handleError(err, -1, "Failed updating status message") {
					mID = nID
				}
			}

		}
	}
}

func (m *Manager) updateStatusMessage(mID string) (string, error) {
	content := ""

	status := m.GetFullStatus()
	for _, shard := range status.Shards {
		emoji := ""
		if shard.OK {
			emoji = "👌"
		} else {
			emoji = "🔥"
		}
		content += fmt.Sprintf("[%d/%d]: %s (%d,%d)\n", shard.Shard+1, len(status.Shards), emoji, shard.Guilds, status.NumGuilds)
	}

	content += "\n\nLast updated(UTC): " + time.Now().UTC().Format(time.RFC822)

	embed := &discordgo.MessageEmbed{
		Title:       "Shard statuses",
		Description: content,
		Color:       0x4286f4,
	}

	if mID == "" {
		msg, err := m.bareSession.ChannelMessageSendEmbed(m.LogChannel, embed)
		if err != nil {
			return "", err
		}

		return msg.ID, err
	}

	_, err := m.bareSession.ChannelMessageEditEmbed(m.LogChannel, mID, embed)
	return mID, err
}

func (m *Manager) GetFullStatus() *Status {
	guilds := m.GuildsProvider.ConnectedGuilds()

	m.RLock()

	result := make([]*ShardStatus, len(m.Sessions))
	for i, shard := range m.Sessions {
		result[i] = &ShardStatus{
			Shard: i,
		}

		if shard == nil {
			result[i].OK = false
		} else {
			shard.RLock()
			result[i].OK = shard.DataReady
			shard.RUnlock()
		}
	}
	m.RUnlock()

	for _, id := range guilds {
		parsed, _ := strconv.ParseInt(id, 10, 64)
		shardID := (parsed >> 22) % int64(len(result))
		result[shardID].Guilds++
	}

	return &Status{
		Shards:    result,
		NumGuilds: len(guilds),
	}
}

type Status struct {
	Shards    []*ShardStatus
	NumGuilds int
}

type ShardStatus struct {
	Shard  int
	OK     bool
	Guilds int
}

// The standard connected guilds provider
type StdGuildsProvider struct {
	Manager *Manager
}

func (s *StdGuildsProvider) ConnectedGuilds() []string {

	s.Manager.RLock()
	result := make([]string, 0, 100)

	for _, v := range s.Manager.Sessions {
		v.State.RLock()
		for _, g := range v.State.Guilds {
			result = append(result, g.ID)
		}
		v.State.RUnlock()
	}

	s.Manager.RUnlock()

	return result
}

// Event holds data for an event
type Event struct {
	Type EventType

	Shard     int
	NumShards int

	Msg string

	// When this event occured
	Time time.Time
}

func (c *Event) String() string {
	prefix := ""
	if c.Shard > -1 {
		prefix = fmt.Sprintf("[%d/%d] ", c.Shard+1, c.NumShards)
	}

	s := fmt.Sprintf("%s%s", prefix, strings.Title(c.Type.String()))
	if c.Msg != "" {
		s += ": " + c.Msg
	}

	return s
}

type EventType int

const (
	// Sent when the connection to the gateway was established
	EventConnected EventType = iota

	// Sent when the connection is lose
	EventDisconnected

	// Sent when the connection was sucessfully resumed
	EventResumed

	// Sent on ready
	EventReady

	// Sent when Open() is called
	EventOpen

	// Send when Close() is called
	EventClose

	// Sent when an error occurs
	EventError
)

var (
	eventStrings = map[EventType]string{
		EventOpen:         "opened",
		EventClose:        "closed",
		EventConnected:    "connected",
		EventDisconnected: "disconnected",
		EventResumed:      "resumed",
		EventReady:        "ready",
		EventError:        "error",
	}
)

func (c EventType) String() string {
	return eventStrings[c]
}
