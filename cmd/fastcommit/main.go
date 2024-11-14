package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"al.essio.dev/pkg/shellescape"
	fastcommit "github.com/AkhilSharma90/GenAI-Code-Committer"
	"github.com/sashabaranov/go-openai"
)

// Command line flags
type flags struct {
	openAIKey     string
	openAIBaseURL string
	model         string
	saveKey       bool
	dryRun        bool
	amend         bool
	context       arrayFlags
}

// Custom type to handle multiple --context flags
type arrayFlags []string

func (i *arrayFlags) String() string {
	return strings.Join(*i, ",")
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

var debugMode = os.Getenv("FASTCOMMIT_DEBUG") != ""

func debugf(format string, args ...any) {
	if !debugMode {
		return
	}
	fmt.Fprintf(os.Stderr, "\033[90mdebug: "+format+"\n\033[0m", args...)
}

func errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\033[31merr: "+format+"\033[0m", args...)
}

func getLastCommitHash() (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func resolveRef(ref string) (string, error) {
	cmd := exec.Command("git", "rev-parse", ref)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func formatShellCommand(cmd *exec.Cmd) string {
	buf := &strings.Builder{}
	buf.WriteString(filepath.Base(cmd.Path))
	for _, arg := range cmd.Args[1:] {
		buf.WriteString(" ")
		buf.WriteString(shellescape.Quote(arg))
	}
	return buf.String()
}

func cleanAIMessage(msg string) string {
	if strings.HasPrefix(msg, "```") {
		msg = strings.TrimSuffix(msg, "```")
		msg = strings.TrimPrefix(msg, "```")
	}
	msg = strings.TrimSpace(msg)
	return msg
}

func run(f flags, ref string) error {
	workdir, err := os.Getwd()
	if err != nil {
		return err
	}

	if ref != "" && f.amend {
		return errors.New("cannot use both [ref] and --amend")
	}

	hash := ""
	if f.amend {
		lastCommitHash, err := getLastCommitHash()
		if err != nil {
			return err
		}
		hash = lastCommitHash
	} else if ref != "" {
		hash, err = resolveRef(ref)
		if err != nil {
			return fmt.Errorf("resolve ref %q: %w", ref, err)
		}
	}

	msgs, err := fastcommit.BuildPrompt(os.Stdout, workdir, hash, f.amend, 128000)
	if err != nil {
		return err
	}

	if len(f.context) > 0 {
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleSystem,
			Content: "The user has provided additional context that MUST be" +
				" included in the commit message",
		})
		for _, context := range f.context {
			msgs = append(msgs, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: context,
			})
		}
	}

	if debugMode {
		for _, msg := range msgs {
			debugf("%s: (%v tokens)\n %s\n\n", msg.Role, fastcommit.CountTokens(msg), msg.Content)
		}
		debugf("prompt includes %d commits\n", len(msgs)/2)
	}

	oaiConfig := openai.DefaultConfig(f.openAIKey)
	oaiConfig.BaseURL = f.openAIBaseURL
	client := openai.NewClientWithConfig(oaiConfig)

	// Create context with cancel
	ctx := context.Background()

	stream, err := client.CreateChatCompletionStream(
		ctx,
		openai.ChatCompletionRequest{
			Model:       f.model,
			Stream:      true,
			Temperature: 0,
			StreamOptions: &openai.StreamOptions{
				IncludeUsage: true,
			},
			Messages: msgs,
		})
	if err != nil {
		return err
	}
	defer stream.Close()

	msg := &bytes.Buffer{}

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				debugf("stream EOF")
				break
			}
			return err
		}
		if resp.Usage != nil {
			debugf("total tokens: %d", resp.Usage.TotalTokens)
			break
		}
		c := resp.Choices[0].Delta.Content
		msg.WriteString(c)
		fmt.Printf("\033[34m%s\033[0m", c)
	}
	fmt.Println()

	msg = bytes.NewBufferString(cleanAIMessage(msg.String()))

	cmd := exec.Command("git", "commit", "-m", msg.String())
	if f.amend {
		cmd.Args = append(cmd.Args, "--amend")
	}

	if f.dryRun {
		fmt.Printf("Run the following command to commit:\n%s\n", formatShellCommand(cmd))
		return nil
	}
	if ref != "" {
		debugf("targetting old ref, not committing")
		return nil
	}

	fmt.Println()

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func main() {
	f := flags{}

	flag.StringVar(&f.openAIKey, "openai-key", os.Getenv("OPENAI_API_KEY"), "The OpenAI API key to use")
	flag.StringVar(&f.openAIBaseURL, "openai-base-url", "https://api.openai.com/v1", "The base URL to use for the OpenAI API")
	flag.StringVar(&f.model, "model", "gpt-4o-2024-08-06", "The model to use, e.g. gpt-4o or gpt-4o-mini")
	flag.BoolVar(&f.saveKey, "save-key", false, "Save the OpenAI API key to persistent local configuration and exit")
	flag.BoolVar(&f.dryRun, "dry", false, "Dry run the command")
	flag.BoolVar(&f.amend, "amend", false, "Amend the last commit")
	flag.Var(&f.context, "context", "Extra context beyond the diff to consider when generating the commit message")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [ref]\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Printf("fastcommit %s\n", Version)
		return
	}

	savedKey, err := loadKey()
	if err != nil && !os.IsNotExist(err) {
		errorf("%v\n", err)
		os.Exit(1)
	}

	if savedKey != "" && f.openAIKey == os.Getenv("OPENAI_API_KEY") {
		f.openAIKey = savedKey
	}

	if f.openAIKey == "" {
		errorf("$OPENAI_API_KEY is not set\n")
		os.Exit(1)
	}

	if f.saveKey {
		err := saveKey(f.openAIKey)
		if err != nil {
			errorf("%v\n", err)
			os.Exit(1)
		}

		kp, err := keyPath()
		if err != nil {
			errorf("%v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Saved OpenAI API key to %s\n", kp)
		return
	}

	ref := ""
	if flag.NArg() > 0 {
		ref = flag.Arg(0)
	}

	if err := run(f, ref); err != nil {
		errorf("%v\n", err)
		os.Exit(1)
	}
}
