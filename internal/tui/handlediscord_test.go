package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDiscordHandleLinkAndKind(t *testing.T) {
	assert.Equal(t, "discord", inferHandleKind("https://discord.com/users/123"))
	assert.Equal(t, "discord", inferHandleKind("https://discordapp.com/users/123"))
	assert.Equal(t, "https://discord.com/users/123456789012345678", linkFor(formRow{kind: rkHandle, key: "discord", value: "123456789012345678"}))
	assert.Equal(t, "https://discord.com/users/1", linkFor(formRow{kind: rkHandle, key: "discord", value: "https://discord.com/users/1"}))
}
