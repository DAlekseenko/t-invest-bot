package sheets

import (
	"context"
	"errors"
	"fmt"
	"time"

	"t-invest-bot/internal/config"
)

const (
	ControlRange = "BOT_CONTROL!A:B"
	ConfigRange  = "BOT_CONFIG!A:S"
)

var (
	ErrRevisionMutated     = errors.New("published revision content mutated")
	ErrUnstablePublication = errors.New("publication changed during read")
)

type Reader interface {
	ReadRange(ctx context.Context, rangeName string) ([][]string, error)
}

type Loader struct {
	reader Reader
	now    func() time.Time
}

func NewLoader(reader Reader) *Loader {
	return &Loader{reader: reader, now: time.Now}
}

func (loader *Loader) PublishedSnapshot(ctx context.Context) (config.Snapshot, error) {
	if loader == nil || loader.reader == nil {
		return config.Snapshot{}, errors.New("sheets reader is required")
	}
	loadedAt := loader.now().UTC()
	first, err := loader.readSnapshot(ctx, loadedAt)
	if err != nil {
		return config.Snapshot{}, fmt.Errorf("read first published snapshot: %w", err)
	}
	second, err := loader.readSnapshot(ctx, loadedAt)
	if err != nil {
		return config.Snapshot{}, fmt.Errorf("read confirmation snapshot: %w", err)
	}

	firstRevision := first.Control().PublishedRevision
	secondRevision := second.Control().PublishedRevision
	if firstRevision != secondRevision {
		return config.Snapshot{}, fmt.Errorf(
			"%w: revision changed from %d to %d",
			ErrUnstablePublication,
			firstRevision,
			secondRevision,
		)
	}
	if first.Hash() != second.Hash() {
		return config.Snapshot{}, fmt.Errorf("%w: revision %d", ErrRevisionMutated, firstRevision)
	}
	return second, nil
}

func (loader *Loader) readSnapshot(ctx context.Context, now time.Time) (config.Snapshot, error) {
	controlRows, err := loader.reader.ReadRange(ctx, ControlRange)
	if err != nil {
		return config.Snapshot{}, fmt.Errorf("read %s: %w", ControlRange, err)
	}
	control, err := parseControl(controlRows, now)
	if err != nil {
		return config.Snapshot{}, err
	}

	configRows, err := loader.reader.ReadRange(ctx, ConfigRange)
	if err != nil {
		return config.Snapshot{}, fmt.Errorf("read %s: %w", ConfigRange, err)
	}
	levels, err := parseLevels(configRows, control.PublishedRevision)
	if err != nil {
		return config.Snapshot{}, err
	}
	snapshot, err := config.NewSnapshot(control, levels)
	if err != nil {
		return config.Snapshot{}, fmt.Errorf("build immutable snapshot: %w", err)
	}
	return snapshot, nil
}
