// Copyright (c) 2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package keyprivate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/sylabs/singularity/e2e/internal/keyexec"
	"github.com/sylabs/singularity/internal/pkg/test"
)

type testingEnv struct {
	// base env for running tests
	CmdPath     string `split_words:"true"`
	TestDir     string `split_words:"true"`
	RunDisabled bool   `default:"false"`
}

var testenv testingEnv
var keyPath string
var defaultKeyFile string

func testPrivateKeyNewPair(t *testing.T) {
	tests := []struct {
		name    string
		user    string
		email   string
		note    string
		psk1    string
		psk2    string
		push    bool
		succeed bool
	}{
		{
			name:    "newpair",
			user:    "genbye2etests\n",
			email:   "westley@sylabs.io\n",
			note:    "test key generated by e2e tests\n",
			psk1:    "e2etests\n",
			push:    false,
			succeed: true,
		},
		{
			name:  "newpair",
			user:  "genbye2etests\n",
			email: "westley@sylabs.io\n",
			note:  "test key generated by e2e tests\n",
			psk1:  "e2etests\n",
			//psk2:    "e2etest\n",
			push:    false,
			succeed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			t.Run("remove_private_keyring_before_newpair", test.WithoutPrivilege(func(t *testing.T) { keyexec.RemoveKeyring(t) }))
			c, b, err := keyexec.KeyNewPair(t, tt.user, tt.email, tt.note, tt.psk1, tt.psk2, tt.push)
			if tt.succeed {
				if err != nil {
					t.Log("Command that failed: ", c)
					t.Log(string(b))
					t.Fatalf("unepexted failure: %v", err)
				}
				//t.Run("testing_new_key_from_newpair", keyexec.QuickTestKey)
				keyexec.QuickTestKey(t)
			} else {
				if err == nil {
					t.Log(string(b))
					t.Fatalf("unexpected succees running: %v", err)
				}
			}
		}))
	}
}

func testPrivateKey(t *testing.T) {
	tests := []struct {
		name    string
		stdin   string
		file    string
		armor   bool
		corrupt bool
		succeed bool
	}{
		{
			name:    "export private",
			armor:   false,
			stdin:   "0\n", // TODO: this will need to be '1' at some point in time -> issue #3199
			file:    defaultKeyFile,
			succeed: true,
		},
		{
			name:    "export private armor",
			armor:   true,
			stdin:   "0\n", // TODO: this will need to be '1' at some point in time -> issue #3199
			file:    defaultKeyFile,
			succeed: true,
		},
		{
			name:    "export private armor corrupt",
			armor:   true,
			stdin:   "0\n", // TODO: this will need to be '1' at some point in time -> issue #3199
			file:    defaultKeyFile,
			corrupt: true,
			succeed: false,
		},
		{
			name:    "export private panic",
			armor:   false,
			stdin:   "1\n", // TODO: this will need to be '1' at some point in time -> issue #3199
			file:    defaultKeyFile,
			succeed: false,
		},
		{
			name:    "export private armor panic",
			armor:   true,
			stdin:   "1\n", // TODO: this will need to be '1' at some point in time -> issue #3199
			file:    defaultKeyFile,
			succeed: false,
		},
		{
			name:    "export private armor invalid",
			armor:   true,
			stdin:   "n\n", // TODO: this will need to be '1' at some point in time -> issue #3199
			file:    defaultKeyFile,
			succeed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, test.WithoutPrivilege(func(t *testing.T) {
			os.RemoveAll(defaultKeyFile)
			c, b, err := keyexec.ExportPrivateKey(t, tt.file, tt.stdin, tt.armor)

			switch {
			case tt.succeed && err == nil:
				// MAYBE PASS: expecting success, succeeded
				t.Run("remove_private_keyring_before_importing", test.WithoutPrivilege(func(t *testing.T) { keyexec.RemoveSecretKeyring(t) }))
				t.Run("import_private_keyring_from", test.WithoutPrivilege(func(t *testing.T) {
					c, b, err := keyexec.ImportPrivateKey(t, defaultKeyFile)
					if err != nil {
						t.Log("command that failed: ", c, string(b))
						t.Fatalf("Unable to import key: %v", err)
					}
				}))

			case !tt.succeed && err != nil:
				// PASS: expecting failure, failed

			case tt.succeed && err != nil:
				// FAIL: expecting success, failed

				t.Logf("Running command:\n%s\nOutput:\n%s\n", c, string(b))
				t.Fatalf("Unexpected failure: %v", err)

			case !tt.succeed && err == nil:
				// FAIL: expecting failure, succeeded
				if tt.corrupt {
					t.Run("corrupting_key", test.WithoutPrivilege(func(t *testing.T) { keyexec.CorruptKey(t, defaultKeyFile) }))
					t.Run("import_private_key", test.WithoutPrivilege(func(t *testing.T) {
						c, b, err := keyexec.ImportPrivateKey(t, defaultKeyFile)
						if err == nil {
							t.Fatalf("Unexpected success: running: %s, %s", c, string(b))
						}
					}))
				} else {
					t.Logf("Running command:\n%s\nOutput:\n%s\n", c, string(b))
					t.Fatalf("Unexpected success: command should have failed: %s, %s", c, string(b))
				}
			}
		}))
	}
}

func TestAll(t *testing.T) {
	err := envconfig.Process("E2E", &testenv)
	if err != nil {
		t.Fatal(err.Error())
	}

	t.Run("importing_test_key", test.WithoutPrivilege(func(t *testing.T) {
		c, b, err := keyexec.ImportPrivateKey(t, "./key/testdata/e2e_test_key.asc")
		if err != nil {
			t.Log("command that failed: ", c, string(b))
			t.Fatalf("Unable to import test key: %v", err)
		}
	}))

	keyPath = testenv.TestDir
	defaultKeyFile = filepath.Join(keyPath, "exported_private_key")

	// Run the tests
	t.Run("private_key", testPrivateKey)
	t.Run("newpair_key_test", testPrivateKeyNewPair)
}
