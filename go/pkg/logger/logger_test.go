package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Enabled is what decides whether the pgx query tracer is installed at all, so
// a level that is not debug has to answer false for LevelDebug.
func TestEnabled_FollowsTheConfiguredLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		configured string
		want       map[Level]bool
	}{
		{configured: "debug", want: map[Level]bool{LevelDebug: true, LevelInfo: true, LevelWarn: true, LevelError: true}},
		{configured: "info", want: map[Level]bool{LevelDebug: false, LevelInfo: true, LevelWarn: true, LevelError: true}},
		{configured: "warn", want: map[Level]bool{LevelDebug: false, LevelInfo: false, LevelWarn: true, LevelError: true}},
		{configured: "error", want: map[Level]bool{LevelDebug: false, LevelInfo: false, LevelWarn: false, LevelError: true}},
		{configured: "nonsense", want: map[Level]bool{LevelDebug: false, LevelInfo: true, LevelWarn: true, LevelError: true}},
	}

	for _, tt := range tests {
		t.Run(tt.configured, func(t *testing.T) {
			t.Parallel()

			log := New(LoggingConfig{Level: tt.configured, Format: "text"})

			for level, want := range tt.want {
				assert.Equal(t, want, log.Enabled(level), "level %d", level)
			}
		})
	}
}

func TestEnabled_IsAlwaysFalseOnTheNoOpLogger(t *testing.T) {
	t.Parallel()

	log := NewNoOpLogger()

	for _, level := range []Level{LevelDebug, LevelInfo, LevelWarn, LevelError} {
		assert.False(t, log.Enabled(level), "level %d", level)
	}
}
