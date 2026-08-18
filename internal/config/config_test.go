package config

import (
	"testing"

	"github.com/lwmacct/251207-go-pkg-cfgm/pkg/cfgm"
)

var files = cfgm.ConfigFiles[Config]{
	Manager:     Manager,
	ExampleFile: "config/config.example.yaml",
	RuntimeFile: "config/config.yaml",
}

func TestWriteConfigExample(t *testing.T) { files.WriteExample(t) }
func TestRuntimeConfigKeysValid(t *testing.T) {
	t.Setenv("DIRECTIVE_HMAC_SECRET", "test-hmac-secret")
	files.ValidateRuntimeConfig(t)
}
