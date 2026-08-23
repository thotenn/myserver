package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thotenn/myserver/internal/config"
	"go.uber.org/zap"
)

// Validate reports whether every config file parses.
//
// It re-reads from disk through config.ValidateFromDisk rather than asking the
// cached loaders, which answer from a snapshot whose load errors were
// discarded — a cache-backed check reports a broken file as valid.
//
// The parse error itself is useful to the operator ("line 12: did not find
// expected key"), so it is kept — but the absolute path is not. Loader errors
// wrap os.ReadFile, whose message carries the full config-dir path, and
// handlers must not leak filesystem layout. The full error goes to the log.
//
// auth.yaml is deliberately absent from this report: its validation errors
// name environment variables, which would turn a config check into an
// environment probe. Its state is reported in the logs instead.
func Validate(logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := dashboardOf(w, r)
		if !ok {
			return
		}
		// Read from disk, not from the cache: the cache swallows loader
		// errors, so cached answers would report a broken file as valid.
		failures := config.ValidateFromDisk(d.Dir)

		files := make([]string, 0, len(failures))
		for file := range failures {
			files = append(files, file)
		}
		sort.Strings(files) // stable output for humans and diffs

		errors := make([]string, 0, len(failures))
		for _, file := range files {
			err := failures[file]
			logger.Warn("config validation failed",
				zap.String("file", file), zap.Error(err))
			errors = append(errors, file+": "+scrubConfigPaths(d.Dir, err.Error()))
		}

		w.Header().Set("Content-Type", "application/json")
		if len(errors) > 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"valid":  false,
				"errors": errors,
			})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"valid": true,
		})
	}
}

// scrubConfigPaths removes filesystem paths from a message, keeping the part
// that helps the operator fix their YAML.
//
// It replaces the config directory with nothing ("/app/config/services.yaml"
// becomes "services.yaml") and drops any remaining absolute path, so the
// response never describes where the container keeps its files.
func scrubConfigPaths(dir, msg string) string {
	if dir != "" {
		msg = strings.ReplaceAll(msg, dir+string(filepath.Separator), "")
		msg = strings.ReplaceAll(msg, dir, "")
	}

	// Anything still starting with a separator is a path we did not expect.
	fields := strings.Fields(msg)
	for i, f := range fields {
		trimmed := strings.TrimRight(f, ":,;")
		if strings.HasPrefix(trimmed, string(filepath.Separator)) && len(trimmed) > 1 {
			fields[i] = strings.Replace(f, trimmed, filepath.Base(trimmed), 1)
		}
	}
	return strings.Join(fields, " ")
}
