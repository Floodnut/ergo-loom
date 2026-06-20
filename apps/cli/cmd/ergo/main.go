package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jkj-dev/ergo-loom/internal/storage/sqlitecli"
	"github.com/jkj-dev/ergo-loom/internal/web"
)

const defaultDataDirName = ".ergo-loom"
const defaultDBFile = "local.db"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ergo:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	store := defaultStore()

	switch args[0] {
	case "init":
		if err := store.Init(); err != nil {
			return err
		}
		fmt.Printf("initialized %s\n", store.DBPath)
		return nil
	case "app", "web":
		return runApp(store, args[1:])
	case "import":
		return runImport(args[1:])
	case "sessions":
		if err := store.Init(); err != nil {
			return err
		}
		sessions, err := store.ListSessions()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("no sessions")
			return nil
		}
		for _, session := range sessions {
			fmt.Printf("%s\t%s\t%s\t%s\n", session.ID, session.SourceTool, session.UpdatedAt.Format("2006-01-02 15:04"), session.Title)
		}
		return nil
	case "show":
		if len(args) != 2 {
			return errors.New("usage: ergo show <session-id>")
		}
		if err := store.Init(); err != nil {
			return err
		}
		session, messages, err := store.GetSession(args[1])
		if err != nil {
			return err
		}
		fmt.Printf("%s\n%s %s\n\n", session.Title, session.ID, session.SourceTool)
		for _, message := range messages {
			fmt.Printf("[%s] %s\n\n", message.Role, strings.TrimSpace(message.Content))
		}
		return nil
	case "branch":
		return runBranch(store, args[1:])
	case "providers":
		if err := store.Init(); err != nil {
			return err
		}
		items, err := store.ListProviderPlugins()
		if err != nil {
			return err
		}
		printRegistry("no providers", items)
		return nil
	case "profiles":
		if err := store.Init(); err != nil {
			return err
		}
		profiles, err := store.ListProviderProfiles()
		if err != nil {
			return err
		}
		if len(profiles) == 0 {
			fmt.Println("no provider profiles")
			return nil
		}
		for _, profile := range profiles {
			defaultMark := ""
			if profile.IsDefault {
				defaultMark = " default"
			}
			fmt.Printf("%s\t%s\t%s%s\n", profile.ID, profile.ProviderPluginID, profile.DisplayName, defaultMark)
		}
		return nil
	case "routes":
		if err := store.Init(); err != nil {
			return err
		}
		routes, err := store.ListAccessRoutes()
		if err != nil {
			return err
		}
		printRoutes(routes)
		return nil
	case "agents":
		if err := store.Init(); err != nil {
			return err
		}
		items, err := store.ListAgentPlugins()
		if err != nil {
			return err
		}
		printRegistry("no agents", items)
		return nil
	case "capabilities":
		if err := store.Init(); err != nil {
			return err
		}
		items, err := store.ListCapabilities()
		if err != nil {
			return err
		}
		printRegistry("no capabilities", items)
		return nil
	case "usage":
		return runUsage(store, args[1:])
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runImport(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: ergo import <codex|copilot|cursor|claude|gemini>")
	}

	switch args[0] {
	case "codex", "copilot", "cursor", "claude", "gemini":
		return fmt.Errorf("import adapter %q is not implemented yet", args[0])
	default:
		return fmt.Errorf("unknown source tool %q", args[0])
	}
}

func defaultStore() sqlitecli.Store {
	dbPath := strings.TrimSpace(os.Getenv("ERGO_LOOM_DB_PATH"))
	if dbPath == "" {
		dataDir := strings.TrimSpace(os.Getenv("ERGO_LOOM_DATA_DIR"))
		if dataDir == "" {
			homeDir, err := os.UserHomeDir()
			if err == nil && strings.TrimSpace(homeDir) != "" {
				dataDir = filepath.Join(homeDir, defaultDataDirName)
			}
		}
		if dataDir == "" {
			dbPath = filepath.Join("data", defaultDBFile)
		} else {
			dbPath = filepath.Join(dataDir, defaultDBFile)
		}
	}

	store := sqlitecli.New(dbPath)
	if appRoot := strings.TrimSpace(os.Getenv("ERGO_LOOM_APP_ROOT")); appRoot != "" {
		store.SchemaPath = filepath.Join(appRoot, "internal", "storage", "sqlitecli", "schema.sql")
	}
	return store
}

func runApp(store sqlitecli.Store, args []string) error {
	flags := flag.NewFlagSet("app", flag.ContinueOnError)
	addr := flags.String("addr", "127.0.0.1:3763", "address for the local chat app")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: ergo app [--addr 127.0.0.1:3763]")
	}
	if err := store.Init(); err != nil {
		return err
	}

	server := web.NewServer(store)
	fmt.Printf("Ergo Loom chat app: http://%s\n", *addr)
	return http.ListenAndServe(*addr, server.Handler())
}

func runBranch(store sqlitecli.Store, args []string) error {
	flags := flag.NewFlagSet("branch", flag.ContinueOnError)
	fromMessageID := flags.String("from", "", "message id to branch from")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || *fromMessageID == "" {
		return errors.New("usage: ergo branch <session-id> --from <message-id>")
	}

	if err := store.Init(); err != nil {
		return err
	}
	session, err := store.CreateBranch(flags.Arg(0), *fromMessageID)
	if err != nil {
		return err
	}
	fmt.Printf("created branch session %s\n", session.ID)
	return nil
}

func runUsage(store sqlitecli.Store, args []string) error {
	if len(args) > 0 && args[0] == "add" {
		return runUsageAdd(store, args[1:])
	}
	if len(args) != 0 {
		return errors.New("usage: ergo usage [add --provider <id> --model <model> --prompt <n> --completion <n>]")
	}

	if err := store.Init(); err != nil {
		return err
	}
	summaries, err := store.TokenUsageSummary()
	if err != nil {
		return err
	}
	if len(summaries) == 0 {
		fmt.Println("no token usage recorded")
		return nil
	}
	for _, summary := range summaries {
		profile := summary.ProviderProfileID
		if profile == "" {
			profile = "-"
		}
		total := summary.PromptTokens + summary.CompletionTokens
		fmt.Printf("%s\t%s\t%s\trequests=%d\tprompt=%d\tcompletion=%d\ttotal=%d\n",
			summary.ProviderPluginID,
			profile,
			summary.Model,
			summary.Requests,
			summary.PromptTokens,
			summary.CompletionTokens,
			total,
		)
	}
	return nil
}

func runUsageAdd(store sqlitecli.Store, args []string) error {
	flags := flag.NewFlagSet("usage add", flag.ContinueOnError)
	provider := flags.String("provider", "", "provider plugin id")
	profile := flags.String("profile", "", "provider profile id")
	model := flags.String("model", "", "model name")
	prompt := flags.String("prompt", "0", "prompt tokens")
	completion := flags.String("completion", "0", "completion tokens")
	session := flags.String("session", "", "session id")
	status := flags.String("status", "success", "usage status")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: ergo usage add --provider <id> --model <model> --prompt <n> --completion <n>")
	}

	promptTokens, err := strconv.Atoi(*prompt)
	if err != nil {
		return fmt.Errorf("invalid prompt token count %q", *prompt)
	}
	completionTokens, err := strconv.Atoi(*completion)
	if err != nil {
		return fmt.Errorf("invalid completion token count %q", *completion)
	}
	if promptTokens < 0 || completionTokens < 0 {
		return errors.New("token counts must be zero or greater")
	}

	if err := store.Init(); err != nil {
		return err
	}
	if err := store.AddTokenUsage(sqlitecli.TokenUsageInput{
		ProviderPluginID:  *provider,
		ProviderProfileID: *profile,
		SessionID:         *session,
		Model:             *model,
		PromptTokens:      promptTokens,
		CompletionTokens:  completionTokens,
		Status:            *status,
	}); err != nil {
		return err
	}
	fmt.Println("recorded token usage")
	return nil
}

func printRegistry(empty string, items []sqlitecli.RegistryItem) {
	if len(items) == 0 {
		fmt.Println(empty)
		return
	}
	for _, item := range items {
		status := "disabled"
		if item.Enabled {
			status = "enabled"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", item.ID, item.Kind, status, item.DisplayName)
	}
}

func printRoutes(routes []sqlitecli.AccessRoute) {
	if len(routes) == 0 {
		fmt.Println("no access routes")
		return
	}
	for _, route := range routes {
		flags := make([]string, 0, 4)
		if route.RequiresLicense {
			flags = append(flags, "license")
		}
		if route.RequiresAPIKey {
			flags = append(flags, "api-key")
		}
		if route.SupportsStreaming {
			flags = append(flags, "streaming")
		}
		if route.SupportsHandoff {
			flags = append(flags, "handoff")
		}
		if len(flags) == 0 {
			flags = append(flags, "no-extra-auth")
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			route.ID,
			route.ProviderPluginID,
			route.AccessMode,
			route.Transport,
			route.CostModel,
			route.Status,
			strings.Join(flags, ","),
		)
	}
}

func printUsage() {
	fmt.Println(`ergo manages local AI work context.

Usage:
  ergo init
  ergo app
  ergo import <codex|copilot|cursor|claude|gemini>
  ergo sessions
  ergo show <session-id>
  ergo branch <session-id> --from <message-id>
  ergo providers
  ergo profiles
  ergo routes
  ergo agents
  ergo capabilities
  ergo usage
  ergo usage add --provider <id> --model <model> --prompt <n> --completion <n>`)
}
