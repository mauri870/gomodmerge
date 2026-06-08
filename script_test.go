package gomodmerge_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"rsc.io/script"
	"rsc.io/script/scripttest"
)

func TestAll(t *testing.T) {
	ctx := context.Background()

	engine := &script.Engine{
		Conds: scripttest.DefaultConds(),
		Cmds:  scriptCmds(),
		Quiet: !testing.Verbose(),
	}
	env := os.Environ()
	// make sure we have the commands installed
	cmd := exec.Command("go", "install", "./cmd/gomodmerge")
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to install commands: %v:\n%s", err, out)
	}
	scripttest.Test(t, ctx, engine, env, "testdata/*.txt")
}

func scriptCmds() map[string]script.Cmd {
	cmds := scripttest.DefaultCmds()
	cmds["gomodmerge"] = script.Program("gomodmerge", nil, 0)
	cmds["go"] = script.Program("go", nil, 0)
	cmds["git"] = script.Program("git", nil, 0)
	return cmds
}
