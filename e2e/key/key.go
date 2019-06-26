// Copyright (c) 2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package key

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/sylabs/singularity/e2e/internal/keyexec"
	"github.com/sylabs/singularity/e2e/key/keyprivate"
	"github.com/sylabs/singularity/e2e/key/keypublic"
	"github.com/sylabs/singularity/internal/pkg/test"
)

type testingEnv struct {
	// base env for running tests
	CmdPath     string `split_words:"true"`
	TestDir     string `split_words:"true"`
	RunDisabled bool   `default:"false"`
}

var testenv testingEnv

func testGeneralKeyCmd(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		succeed bool
	}{
		{
			name:    "key list",
			args:    []string{"list"},
			succeed: true,
		},
		{
			name:    "key list secret",
			args:    []string{"list", "-s"},
			succeed: true,
		},
		{
			name:    "key list bad flag",
			args:    []string{"list", "--not-a-flag"},
			succeed: false,
		},
		{
			name:    "key bad cmd",
			args:    []string{"notacmd"},
			succeed: false,
		},
		{
			name:    "key bad cmd flag",
			args:    []string{"notacmd", "--bad"},
			succeed: false,
		},
		{
			name:    "key flag",
			args:    []string{"--notaflag"},
			succeed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			cmd, out, err := keyexec.RunKeyCmd(t, tt.args, "")
			if tt.succeed {
				if err != nil {
					t.Log("Command that failed: ", cmd)
					t.Log(string(out))
					t.Fatalf("Unexpected failure: %v", err)
				}
			} else {
				if err == nil {
					t.Log("Command that succeed: ", cmd)
					t.Log(string(out))
					t.Fatalf("Unexpected success: %v", err)
				}
			}
		}))
	}
}

// RunE2ETests is the main func to trigger the test suite
func RunE2ETests(t *testing.T) {
	err := envconfig.Process("E2E", &testenv)
	if err != nil {
		t.Fatal(err.Error())
	}

	os.Setenv("SINGULARITY_SYPGPDIR", filepath.Join(testenv.TestDir, "sypgp_keyring"))

	// Import a public ascii coruppted key
	t.Run("importing_test_key", test.WithoutPrivilege(func(t *testing.T) {
		c, b, err := keyexec.ImportKey(t, "./key/testdata/public_ascii_corrupted_key.asc")
		if err == nil {
			t.Log("command that succeed: ", c, string(b))
			t.Fatalf("Unexpected success, command should fail: %v", c)
		}
	}))

	// Import a private binary key
	t.Run("importing_test_key", test.WithoutPrivilege(func(t *testing.T) {
		c, b, err := keyexec.ImportPrivateKey(t, "./key/testdata/private_key.asc")
		if err != nil {
			t.Log("command that failed: ", c, string(b))
			t.Fatalf("Unable to import test key: %v", err)
		}
	}))

	// Import a private ascii key
	t.Run("importing_test_key", test.WithoutPrivilege(func(t *testing.T) {
		c, b, err := keyexec.ImportPrivateKey(t, "./key/testdata/private_ascii_key.asc")
		if err != nil {
			t.Log("command that failed: ", c, string(b))
			t.Fatalf("Unable to import test key: %v", err)
		}
	}))

	// Import a public binary key
	t.Run("importing_test_key", test.WithoutPrivilege(func(t *testing.T) {
		c, b, err := keyexec.ImportKey(t, "./key/testdata/public_key.asc")
		if err != nil {
			t.Log("command that failed: ", c, string(b))
			t.Fatalf("Unable to import test key: %v", err)
		}
	}))

	// Import a public ascii key
	t.Run("importing_test_key", test.WithoutPrivilege(func(t *testing.T) {
		c, b, err := keyexec.ImportKey(t, "./key/testdata/public_ascii_key.asc")
		if err != nil {
			t.Log("command that failed: ", c, string(b))
			t.Fatalf("Unable to import test key: %v", err)
		}
	}))

	t.Run("GeneralKeyCmdTest", testGeneralKeyCmd)
	t.Run("PublicKey", keypublic.TestAll)
	t.Run("PrivateKey", keyprivate.TestAll)

	os.Unsetenv("SINGULARITY_SYPGPDIR")
}