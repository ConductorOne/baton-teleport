package config

import (
	"testing"

	"github.com/conductorone/baton-sdk/pkg/test"
	"github.com/conductorone/baton-sdk/pkg/ustrings"
)

func TestConfigSchema(t *testing.T) {
	test.ExerciseTestCasesFromExpressions(
		t,
		Config,
		nil,
		ustrings.ParseFlags,
		[]test.TestCaseFromExpression{
			{
				"",
				false,
				"empty config",
			},
			{
				"--teleport-proxy-address 1",
				false,
				"missing private key",
			},
			{
				"--teleport-proxy-address 1 --teleport-key 1 --teleport-key-path 1",
				false,
				"both private key types",
			},
			{
				"--teleport-proxy-address 1 --teleport-key-path 1",
				true,
				"private key path",
			},
			{
				"--teleport-proxy-address 1 --teleport-key 1",
				true,
				"private key",
			},
		},
	)
}
