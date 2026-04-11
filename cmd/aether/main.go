package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"aether/internal/node"
	"aether/internal/protocol"
)

const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
	colorWhite  = "\033[97m"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%serror:%s %v\n", colorRed, colorReset, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	cfg := node.DefaultConfig()

	switch args[0] {
	case "timeline", "tl":
		return runTimeline(cfg, args[1:])
	case "post", "p":
		if len(args) < 2 {
			return fmt.Errorf("usage: aether post <message>")
		}
		return runPost(cfg, args[1:])
	case "status", "s":
		return runStatus(cfg)
	case "peers":
		return runPeers(cfg)
	case "search":
		if len(args) < 2 {
			return fmt.Errorf("usage: aether search <term>")
		}
		return runSearch(cfg, args[1])
	case "export":
		return runExport(cfg)
	case "count":
		return runCount(cfg)
	case "help", "--help", "-h":
		return usage()
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

// --- timeline ---

func runTimeline(cfg node.Config, args []string) error {
	store, err := node.OpenStore(cfg.DataDir)
	if err != nil {
		return err
	}

	jsonMode := hasFlag(args, "--json")
	follow := hasFlag(args, "--follow", "-f")
	limit := flagInt(args, 30, "--limit", "-n")

	if follow {
		return runTimelineFollow(store, jsonMode)
	}

	messages, err := store.LoadAll()
	if err != nil {
		return err
	}

	if len(messages) == 0 {
		fmt.Printf("%sno messages yet%s\n", colorDim, colorReset)
		return nil
	}

	start := len(messages) - limit
	if start < 0 {
		start = 0
	}
	recent := messages[start:]

	if jsonMode {
		return json.NewEncoder(os.Stdout).Encode(recent)
	}

	printHeader(fmt.Sprintf("Timeline · %d messages (showing last %d)", len(messages), len(recent)))
	for _, msg := range recent {
		printMessage(msg)
	}
	return nil
}

func runTimelineFollow(store *node.Store, jsonMode bool) error {
	lastCount := 0
	messages, err := store.LoadAll()
	if err != nil {
		return err
	}
	lastCount = len(messages)

	if !jsonMode {
		printHeader("Timeline · live")
		if len(messages) > 0 {
			start := len(messages) - 10
			if start < 0 {
				start = 0
			}
			for _, msg := range messages[start:] {
				printMessage(msg)
			}
		}
	}

	for {
		time.Sleep(2 * time.Second)
		messages, err = store.LoadAll()
		if err != nil {
			continue
		}
		if len(messages) <= lastCount {
			continue
		}
		for _, msg := range messages[lastCount:] {
			if jsonMode {
				data, _ := json.Marshal(msg)
				fmt.Println(string(data))
			} else {
				printMessage(msg)
			}
		}
		lastCount = len(messages)
	}
}

// --- post ---

func runPost(cfg node.Config, args []string) error {
	target := ""
	content := args[0]

	if hasFlag(args, "--peer") {
		idx := flagIndex(args, "--peer")
		if idx >= 0 && idx+1 < len(args) {
			target = args[idx+1]
			// content is the remaining non-flag arg
			for _, a := range args {
				if a != "--peer" && a != target && !strings.HasPrefix(a, "-") {
					content = a
					break
				}
			}
		}
	} else if len(args) >= 2 && !strings.HasPrefix(args[0], "-") && !strings.HasPrefix(args[1], "-") {
		target = args[0]
		content = args[1]
	}

	if err := node.ValidateTorRuntime(cfg, false); err != nil {
		return err
	}

	peerBook, err := node.OpenPeerBook(cfg.DataDir)
	if err != nil {
		return err
	}
	peerBook.ApplyPolicy(cfg)
	_ = node.Bootstrap(cfg, peerBook)

	msg, err := protocol.NewMessage(content)
	if err != nil {
		return err
	}

	fmt.Printf("%s⛏  mining proof-of-work (difficulty=%d)...%s", colorDim, protocol.Difficulty, colorReset)
	start := time.Now()
	nonce, err := protocol.MinePoW(msg.H, protocol.Difficulty)
	if err != nil {
		fmt.Println()
		return err
	}
	msg.P = nonce
	elapsed := time.Since(start)
	fmt.Printf("\r%s✓  mined in %s%s\n", colorGreen, elapsed.Round(time.Millisecond), colorReset)

	fmt.Printf("%s⇡  relaying to network...%s", colorDim, colorReset)
	if err := node.SendMessage(cfg, peerBook, target, msg); err != nil {
		fmt.Println()
		return fmt.Errorf("relay failed: %w", err)
	}
	fmt.Printf("\r%s✓  message accepted by peer%s\n", colorGreen, colorReset)

	fmt.Println()
	printMessage(*msg)
	return nil
}

// --- status ---

func runStatus(cfg node.Config) error {
	store, err := node.OpenStore(cfg.DataDir)
	if err != nil {
		return err
	}
	messages, err := store.LoadAll()
	if err != nil {
		return err
	}

	peerBook, err := node.OpenPeerBook(cfg.DataDir)
	if err != nil {
		return err
	}
	peers, _ := peerBook.Load()

	printHeader("Aether Node Status")

	fmt.Printf("  %sversion%s      %s\n", colorDim, colorReset, protocol.NodeVersion)
	fmt.Printf("  %sdata dir%s     %s\n", colorDim, colorReset, cfg.DataDir)
	fmt.Printf("  %smessages%s     %s%d%s\n", colorDim, colorReset, colorBold, len(messages), colorReset)
	fmt.Printf("  %speers%s        %s%d%s\n", colorDim, colorReset, colorBold, len(peers), colorReset)
	fmt.Printf("  %stor%s          ", colorDim, colorReset)
	if cfg.DevClearnet {
		fmt.Printf("%sdisabled (dev-clearnet)%s\n", colorYellow, colorReset)
	} else if cfg.RequireTorProxy {
		fmt.Printf("%senabled%s (socks=%s)\n", colorGreen, colorReset, cfg.TorSOCKS)
	} else {
		fmt.Printf("optional\n")
	}
	fmt.Printf("  %sarchive%s      %t\n", colorDim, colorReset, cfg.ArchiveMode)
	fmt.Printf("  %slisten%s       %s\n", colorDim, colorReset, cfg.ListenAddress)
	if cfg.AdvertiseAddr != "" {
		fmt.Printf("  %sadvertise%s    %s\n", colorDim, colorReset, cfg.AdvertiseAddr)
	}

	if len(messages) > 0 {
		last := messages[len(messages)-1]
		fmt.Printf("\n  %slatest msg%s   %s\n", colorDim, colorReset, timeAgo(int64(last.T)))
		fmt.Printf("  %shash%s         %s%.16s…%s\n", colorDim, colorReset, colorCyan, last.H, colorReset)
	}
	fmt.Println()
	return nil
}

// --- peers ---

func runPeers(cfg node.Config) error {
	peerBook, err := node.OpenPeerBook(cfg.DataDir)
	if err != nil {
		return err
	}
	peerBook.ApplyPolicy(cfg)
	peers, err := peerBook.Load()
	if err != nil {
		return err
	}

	if len(peers) == 0 {
		fmt.Printf("%sno peers known%s\n", colorDim, colorReset)
		return nil
	}

	printHeader(fmt.Sprintf("Known Peers (%d)", len(peers)))
	now := time.Now()
	for _, addr := range peers {
		score := peerBook.PenaltyScoreAt(addr, now)
		banned := peerBook.IsBanned(addr, now)
		backed := peerBook.IsBackedOff(addr, now)

		status := fmt.Sprintf("%s●%s", colorGreen, colorReset)
		if banned {
			status = fmt.Sprintf("%s✕ banned%s", colorRed, colorReset)
		} else if backed {
			status = fmt.Sprintf("%s◌ backoff%s", colorYellow, colorReset)
		}

		scoreStr := ""
		if score > 0 {
			scoreStr = fmt.Sprintf(" %s(score=%d)%s", colorDim, score, colorReset)
		}

		fmt.Printf("  %s %s%s\n", status, addr, scoreStr)
	}
	fmt.Println()
	return nil
}

// --- search ---

func runSearch(cfg node.Config, term string) error {
	store, err := node.OpenStore(cfg.DataDir)
	if err != nil {
		return err
	}
	messages, err := store.LoadAll()
	if err != nil {
		return err
	}

	termLower := strings.ToLower(term)
	found := 0
	for _, msg := range messages {
		if strings.Contains(strings.ToLower(msg.C), termLower) {
			printMessage(msg)
			found++
		}
	}

	if found == 0 {
		fmt.Printf("%sno messages matching \"%s\"%s\n", colorDim, term, colorReset)
	} else {
		fmt.Printf("\n%s%d message(s) found%s\n", colorDim, found, colorReset)
	}
	return nil
}

// --- export ---

func runExport(cfg node.Config) error {
	store, err := node.OpenStore(cfg.DataDir)
	if err != nil {
		return err
	}
	messages, err := store.LoadAll()
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(messages)
}

// --- count ---

func runCount(cfg node.Config) error {
	store, err := node.OpenStore(cfg.DataDir)
	if err != nil {
		return err
	}
	messages, err := store.LoadAll()
	if err != nil {
		return err
	}
	fmt.Println(len(messages))
	return nil
}

// --- helpers ---

func printHeader(title string) {
	fmt.Printf("\n  %s%s%s\n", colorBold, title, colorReset)
	fmt.Printf("  %s%s%s\n\n", colorDim, strings.Repeat("─", len(title)+4), colorReset)
}

func printMessage(msg protocol.Message) {
	t := time.Unix(int64(msg.T), 0)
	ago := timeAgo(int64(msg.T))

	fmt.Printf("  %s%s%s %s%s%s\n", colorDim, t.Format("2006-01-02 15:04:05"), colorReset, colorDim, ago, colorReset)
	fmt.Printf("  %s%s%s\n", colorWhite, msg.C, colorReset)
	fmt.Printf("  %s#%.16s…%s\n\n", colorCyan, msg.H, colorReset)
}

func timeAgo(unixSec int64) string {
	diff := time.Since(time.Unix(unixSec, 0))
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		m := int(diff.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", m)
	case diff < 24*time.Hour:
		h := int(diff.Hours())
		if h == 1 {
			return "1 hr ago"
		}
		return fmt.Sprintf("%d hr ago", h)
	default:
		d := int(diff.Hours() / 24)
		if d == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", d)
	}
}

func usage() error {
	fmt.Println()
	fmt.Printf("  %sAether%s — anonymous broadcast protocol\n\n", colorBold, colorReset)
	fmt.Println("  commands:")
	fmt.Printf("    %stimeline%s      show recent messages (--follow, --json, --limit N)\n", colorCyan, colorReset)
	fmt.Printf("    %spost%s          mine + relay a message\n", colorCyan, colorReset)
	fmt.Printf("    %sstatus%s        show node info\n", colorCyan, colorReset)
	fmt.Printf("    %speers%s         list known peers with scores\n", colorCyan, colorReset)
	fmt.Printf("    %ssearch%s        search messages by content\n", colorCyan, colorReset)
	fmt.Printf("    %sexport%s        dump all messages as JSON\n", colorCyan, colorReset)
	fmt.Printf("    %scount%s         print total message count\n", colorCyan, colorReset)
	fmt.Println()
	fmt.Printf("  %saliases: tl=timeline, p=post, s=status%s\n", colorDim, colorReset)
	fmt.Println()
	return nil
}

func hasFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

func flagIndex(args []string, flags ...string) int {
	for i, a := range args {
		for _, f := range flags {
			if a == f {
				return i
			}
		}
	}
	return -1
}

func flagInt(args []string, defaultVal int, flags ...string) int {
	for i, a := range args {
		for _, f := range flags {
			if a == f && i+1 < len(args) {
				v, err := strconv.Atoi(args[i+1])
				if err == nil {
					return v
				}
			}
		}
	}
	return defaultVal
}
