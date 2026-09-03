package cli

import (
	"flag"
	"strings"
)

// parseFlags parses args into fs, permitting flags to appear after positional
// arguments.
//
// The standard flag package stops at the first non-flag argument, so
// `app add Cloudflare "Systems Engineer" -track systems` would silently treat
// the flags as part of the role name. Since every command here takes both
// positionals and flags, that default is a trap. permute reorders args so the
// flags come first, matching what every other CLI does.
func parseFlags(fs *flag.FlagSet, args []string) error {
	return fs.Parse(permute(fs, args))
}

// permute moves flag arguments (and their values) ahead of positional ones.
// A bare "--" ends flag processing, and everything after it stays positional.
func permute(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]

		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(a) < 2 || a[0] != '-' {
			positional = append(positional, a)
			continue
		}

		flags = append(flags, a)

		// "-flag=value" carries its own value.
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		// Booleans never consume the following argument; anything else does.
		f := fs.Lookup(name)
		if f == nil {
			continue // unknown: let flag.Parse report it
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}

	return append(flags, positional...)
}
