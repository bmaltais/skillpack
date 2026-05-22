package main

import (
	"context"

	"github.com/bmaltais/skillpack/internal/config"
	"github.com/bmaltais/skillpack/internal/state"
)

// appKey is the unexported context key type for storing the App.
type appKey struct{}

// App holds the loaded configuration and state, injected into the Cobra
// command context by root.go's PersistentPreRunE.
type App struct {
	Cfg *config.Config
	St  *state.State
}

// AppFromCtx returns the App stored in the Cobra command context.
// Returns nil if no App was injected (e.g. self-update, or a command
// executed outside the normal CLI flow).
func AppFromCtx(ctx context.Context) *App {
	if app, ok := ctx.Value(appKey{}).(*App); ok {
		return app
	}
	return nil
}
