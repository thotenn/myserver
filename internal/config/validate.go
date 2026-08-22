package config

// ValidateFromDisk re-parses every config file from disk and returns the
// errors, keyed by file name.
//
// It deliberately bypasses the in-memory cache. The public loaders answer from
// cachedConfig, which ReloadCache fills with `c.Services, _ = loadServices()` —
// discarding the error and storing nil. A validation endpoint built on those
// loaders therefore reports "valid" for a file it failed to parse, which is
// exactly backwards: the moment the config is broken is the moment validation
// has to speak up.
//
// auth.yaml is not included: its validation errors name environment
// variables, so surfacing them here would turn a config check into an
// environment probe. Its state is reported in the logs instead.
func ValidateFromDisk() map[string]error {
	errs := make(map[string]error)

	if _, err := loadServices(); err != nil {
		errs["services.yaml"] = err
	}
	if _, err := loadBookmarks(); err != nil {
		errs["bookmarks.yaml"] = err
	}
	if _, err := loadWidgets(); err != nil {
		errs["widgets.yaml"] = err
	}
	if _, err := loadSettings(); err != nil {
		errs["settings.yaml"] = err
	}
	if _, err := loadDocker(); err != nil {
		errs["docker.yaml"] = err
	}
	if _, err := loadProxmox(); err != nil {
		errs["proxmox.yaml"] = err
	}
	if _, err := loadScriptsFile(); err != nil {
		errs["scripts.yaml"] = err
	}

	return errs
}
