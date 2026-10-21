// Package config reads the application settings from BIRTHDAY_* environment
// variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Banh-Canh/birthday/internal/countdown"
)

// DefaultMessage is the line shown when BIRTHDAY_MESSAGE is unset.
const DefaultMessage = "I owe you a birthday drink"

// DefaultTeaser hints at the hidden I.O.U. before the birthday when
// BIRTHDAY_TEASER is unset.
const DefaultTeaser = "A little something is waiting for you"

// DefaultOpenDays is how long the I.O.U. stays visible after the birthday
// when BIRTHDAY_OPEN_DAYS is unset.
const DefaultOpenDays = 15

const (
	maxNameLen    = 64
	maxMessageLen = 280
	maxTeaserLen  = 140
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

	Birthday countdown.Birthday
	Location *time.Location

	// OpenDays is how many days after the birthday the I.O.U. stays visible.
	OpenDays int

	// Halloween adds a few quiet seasonal touches (moon, bats, a cobweb).
	Halloween bool
}

// Load builds the configuration from getenv (os.Getenv in production) and
// reports every invalid setting at once.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Host:    getenv("BIRTHDAY_HOST"),
		Port:    valueOr(getenv("BIRTHDAY_PORT"), "8080"),
		Name:    strings.TrimSpace(getenv("BIRTHDAY_NAME")),
		Message: valueOr(strings.TrimSpace(getenv("BIRTHDAY_MESSAGE")), DefaultMessage),
		From:    strings.TrimSpace(getenv("BIRTHDAY_FROM")),
		Teaser:  valueOr(strings.TrimSpace(getenv("BIRTHDAY_TEASER")), DefaultTeaser),
	}

	var errs []error
	if port, err := strconv.Atoi(cfg.Port); err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("BIRTHDAY_PORT %q: want a port number", cfg.Port))
	}
	if cfg.Name == "" {
		errs = append(errs, errors.New("BIRTHDAY_NAME is required"))
	}
	if utf8.RuneCountInString(cfg.Name) > maxNameLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_NAME: at most %d characters", maxNameLen))
	}
	if utf8.RuneCountInString(cfg.Message) > maxMessageLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_MESSAGE: at most %d characters", maxMessageLen))
	}
	if utf8.RuneCountInString(cfg.Teaser) > maxTeaserLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_TEASER: at most %d characters", maxTeaserLen))
	}
	if utf8.RuneCountInString(cfg.From) > maxNameLen {
		errs = append(errs, fmt.Errorf("BIRTHDAY_FROM: at most %d characters", maxNameLen))
	}

	if date := getenv("BIRTHDAY_DATE"); date == "" {
		errs = append(errs, errors.New("BIRTHDAY_DATE is required (MM-DD or YYYY-MM-DD)"))
	} else if b, err := countdown.Parse(date); err != nil {
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
