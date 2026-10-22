// Package config reads the application settings from BIRTHDAY_* environment
// variables.
package config

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Banh-Canh/birthday/internal/countdown"
)

// DefaultName is who the countdown is for when BIRTHDAY_NAME is unset.
const DefaultName = "friend"

// DefaultDate is used when BIRTHDAY_DATE is unset. It is deliberately a fixed
// day rather than "today", so a missing date never opens the surprise early.
const DefaultDate = "01-01"

// DefaultMessage is the line shown when BIRTHDAY_MESSAGE is unset.
const DefaultMessage = "I owe you a birthday drink"

// DefaultTeaser hints at the hidden I.O.U. before the birthday when
// BIRTHDAY_TEASER is unset.
const DefaultTeaser = "A little something is waiting for you"

// DefaultSecretIntro announces BIRTHDAY_SECRET when BIRTHDAY_SECRET_INTRO is
// unset.
const DefaultSecretIntro = "And a little extra, just for you"

// DefaultChatName is who the birthday chat friend is when BIRTHDAY_CHAT_NAME
// is unset.
const DefaultChatName = "Sunny"

// DefaultChatModel is the Claude model the chat friend runs on when
// BIRTHDAY_CHAT_MODEL is unset.
const DefaultChatModel = "sonnet"

// DefaultChatCommand is the Claude Code CLI when BIRTHDAY_CHAT_COMMAND is
// unset.
const DefaultChatCommand = "claude"

// DefaultChatFallback is what the chat friend says, in turn, when Claude
// can't answer (not signed in, usage limits…) and BIRTHDAY_CHAT_FALLBACK is
// unset. {name} becomes BIRTHDAY_NAME.
var DefaultChatFallback = []string{
	"Aww, {name}! 🥳 Today is all about you. I hope it's full of laughter, cake and the people you love!",
	"You deserve the happiest of birthdays, truly. 💛 Never forget how loved you are!",
	"Hehe, I'm grinning just reading that! 😄 Here's to another year of you being wonderful. 🥂",
	"Make a wish, {name}! 🌟 I have a feeling this year is going to be a great one for you.",
	"Sending you the biggest birthday hug from here 🤗🎂",
	"Honestly, the world is lucky to have you. Happy birthday, {name}! 🎉",
}

// DefaultOpenDays is how long the I.O.U. stays visible after the birthday
// when BIRTHDAY_OPEN_DAYS is unset.
const DefaultOpenDays = 15

const (
	maxNameLen    = 64
	maxMessageLen = 280
	maxTeaserLen  = 140
	maxSecretLen  = 500
	minPINLen     = 4
	maxPINLen     = 12
	maxPromptLen  = 8000
	maxFallbacks  = 50
)

// Config is the validated application configuration.
type Config struct {
	Host string
	Port string

	// Name is who the countdown is for, Message the line printed on the
	// I.O.U. card, From an optional signature under it. Teaser is shown in
	// the card's place until it unlocks.
	Name    string
	Message string
	From    string
	Teaser  string

	// Secret is an optional extra revealed with the I.O.U. (e.g. a gift card
	// code or link), meant to come from a secret store; SecretIntro is the
	// line announcing it.
	Secret      string
	SecretIntro string
	// SecretPIN, when set, must be entered to reveal Secret: the page never
	// contains the secret, the server hands it out after checking the PIN.
	SecretPIN string

	Birthday countdown.Birthday
	// DateDefaulted reports that BIRTHDAY_DATE was unset and DefaultDate is
	// in use, so the server can warn about it.
	DateDefaulted bool
	Location      *time.Location

	// OpenDays is how many days after the birthday the I.O.U. stays visible.
	OpenDays int

	// Halloween adds a few quiet seasonal touches (moon, bats, a cobweb).
	Halloween bool

	// Chat adds a chat with a cheerful friend (Claude Code on the login made
	// in the container) to the open card, locked like the secret: only while
	// open, and behind SecretPIN when one is set. ChatName is the friend's
	// name, ChatPrompt extra instructions on how they act, ChatModel the
	// model and ChatCommand the Claude Code CLI.
	Chat        bool
	ChatName    string
	ChatPrompt  string
	ChatModel   string
	ChatCommand string
	// ChatFallback holds the friend's canned replies, used in turn when
	// Claude can't answer.
	ChatFallback []string
}

// Load builds the configuration from getenv (os.Getenv in production) and
// reports every invalid setting at once.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Host:    getenv("BIRTHDAY_HOST"),
		Port:    valueOr(getenv("BIRTHDAY_PORT"), "8080"),
		Name:    valueOr(strings.TrimSpace(getenv("BIRTHDAY_NAME")), DefaultName),
		Message: valueOr(strings.TrimSpace(getenv("BIRTHDAY_MESSAGE")), DefaultMessage),
		From:    strings.TrimSpace(getenv("BIRTHDAY_FROM")),
		Teaser:  valueOr(strings.TrimSpace(getenv("BIRTHDAY_TEASER")), DefaultTeaser),

		Secret:      strings.TrimSpace(getenv("BIRTHDAY_SECRET")),
		SecretIntro: valueOr(strings.TrimSpace(getenv("BIRTHDAY_SECRET_INTRO")), DefaultSecretIntro),
		SecretPIN:   strings.TrimSpace(getenv("BIRTHDAY_SECRET_PIN")),

		ChatName:    valueOr(strings.TrimSpace(getenv("BIRTHDAY_CHAT_NAME")), DefaultChatName),
		ChatPrompt:  strings.TrimSpace(getenv("BIRTHDAY_CHAT_PROMPT")),
		ChatModel:   valueOr(strings.TrimSpace(getenv("BIRTHDAY_CHAT_MODEL")), DefaultChatModel),
		ChatCommand: valueOr(strings.TrimSpace(getenv("BIRTHDAY_CHAT_COMMAND")), DefaultChatCommand),
	}

	var errs []error
	if port, err := strconv.Atoi(cfg.Port); err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("BIRTHDAY_PORT %q: want a port number", cfg.Port))
	}
	if utf8.RuneCountInString(cfg.Name) > maxNameLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_NAME: at most %d characters", maxNameLen))
	}
	if utf8.RuneCountInString(cfg.Message) > maxMessageLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_MESSAGE: at most %d characters", maxMessageLen))
	}
	if utf8.RuneCountInString(cfg.Secret) > maxSecretLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_SECRET: at most %d characters", maxSecretLen))
	}
	if n := utf8.RuneCountInString(cfg.SecretPIN); cfg.SecretPIN != "" && (n < minPINLen || n > maxPINLen) {
		errs = append(errs, fmt.Errorf("BIRTHDAY_SECRET_PIN: %d to %d characters", minPINLen, maxPINLen))
	}
	if utf8.RuneCountInString(cfg.SecretIntro) > maxTeaserLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_SECRET_INTRO: at most %d characters", maxTeaserLen))
	}
	if utf8.RuneCountInString(cfg.Teaser) > maxTeaserLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_TEASER: at most %d characters", maxTeaserLen))
	}
	if utf8.RuneCountInString(cfg.From) > maxNameLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_FROM: at most %d characters", maxNameLen))
	}

	if v := getenv("BIRTHDAY_CHAT"); v != "" {
		if b, err := strconv.ParseBool(v); err != nil {
			errs = append(errs, fmt.Errorf("BIRTHDAY_CHAT %q: want true or false", v))
		} else {
			cfg.Chat = b
		}
	}
	if utf8.RuneCountInString(cfg.ChatName) > maxNameLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_CHAT_NAME: at most %d characters", maxNameLen))
	}
	cfg.ChatFallback = DefaultChatFallback
	if v := strings.TrimSpace(getenv("BIRTHDAY_CHAT_FALLBACK")); v != "" {
		cfg.ChatFallback = nil
		for line := range strings.Lines(v) {
			if line = strings.TrimSpace(line); line != "" {
				cfg.ChatFallback = append(cfg.ChatFallback, line)
			}
		}
		tooLong := func(l string) bool { return utf8.RuneCountInString(l) > maxMessageLen }
		if len(cfg.ChatFallback) > maxFallbacks || slices.ContainsFunc(cfg.ChatFallback, tooLong) {
			errs = append(errs, fmt.Errorf("BIRTHDAY_CHAT_FALLBACK: at most %d lines of %d characters", maxFallbacks, maxMessageLen))
		}
	}
	if utf8.RuneCountInString(cfg.ChatPrompt) > maxPromptLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_CHAT_PROMPT: at most %d characters", maxPromptLen))
	}
	if strings.HasPrefix(cfg.ChatModel, "-") || strings.ContainsAny(cfg.ChatModel, " \t\n") {
		errs = append(errs, fmt.Errorf("BIRTHDAY_CHAT_MODEL %q: want a model name such as sonnet", cfg.ChatModel))
	}

	date := strings.TrimSpace(getenv("BIRTHDAY_DATE"))
	cfg.DateDefaulted = date == ""
	if b, err := countdown.Parse(valueOr(date, DefaultDate)); err != nil {
		errs = append(errs, fmt.Errorf("BIRTHDAY_DATE: %w", err))
	} else {
		cfg.Birthday = b
	}

	cfg.OpenDays = DefaultOpenDays
	if v := getenv("BIRTHDAY_OPEN_DAYS"); v != "" {
		if days, err := strconv.Atoi(v); err != nil || days < 0 || days > 365 {
			errs = append(errs, fmt.Errorf("BIRTHDAY_OPEN_DAYS %q: want a number of days from 0 to 365", v))
		} else {
			cfg.OpenDays = days
		}
	}

	if v := getenv("BIRTHDAY_HALLOWEEN"); v == "" {
		cfg.Halloween = true
	} else if b, err := strconv.ParseBool(v); err != nil {
		errs = append(errs, fmt.Errorf("BIRTHDAY_HALLOWEEN %q: want true or false", v))
	} else {
		cfg.Halloween = b
	}

	tz := valueOr(getenv("BIRTHDAY_TIMEZONE"), "UTC")
	if loc, err := time.LoadLocation(tz); err != nil {
		errs = append(errs, fmt.Errorf("BIRTHDAY_TIMEZONE %q: want an IANA name such as Europe/Paris", tz))
	} else {
		cfg.Location = loc
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
