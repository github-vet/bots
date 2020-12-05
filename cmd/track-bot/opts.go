package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type opts struct {
	GithubToken   string
	TrackingFile  string
	ExpertsFile   string
	GophersFile   string
	Owner         string
	Repo          string
	PollFrequency time.Duration
}

// OptSchema defines a configuration option which can come either from the command-line or
// environment variables.
type OptSchema struct {
	// EnvArgName is the name of the environment variable used for this option.
	EnvArgName string
	// FlagName is the name of a command-line flag used for this option.
	FlagName string
	// FlagUsage is the usage description sent from the flags package for this option.
	FlagUsage string
	// DefaultValue is the value used if no override is given. Empty-string is used for required arguments.
	DefaultValue string
	// Required is true if the value must be set during config.
	Required bool
	// OptSetter is a function run to set the value of the option in the opts struct
	OptSetter func(o *opts, value string) error
}

var optSchemas []OptSchema = []OptSchema{
	{"GITHUB_TOKEN", "token", "GitHub access token", "", true,
		func(o *opts, value string) error { o.GithubToken = value; return nil }},
	{"TRACKING_FILE", "tracking", "path to issue tracking CSV file", "issue_tracking.csv", false,
		func(o *opts, value string) error { o.TrackingFile = value; return nil }},
	{"EXPERTS_FILE", "experts", "path to experts CSV file", "experts.csv", false,
		func(o *opts, value string) error { o.ExpertsFile = value; return nil }},
	{"GOPHERS_FILE", "gophers", "path to gophers CSV file", "gophers.csv", false,
		func(o *opts, value string) error { o.GophersFile = value; return nil }},
	{"GITHUB_REPO", "repo", "owner/repository of GitHub repo where issues should be tracked", "kalexmills/rangeloop-test-repo", false,
		func(o *opts, value string) error {
			repoToks := strings.Split(value, "/")
			if len(repoToks) != 2 {
				return fmt.Errorf("could not parse repo flag '%s' which must be in owner/repository format", value)
			}
			o.Owner = repoToks[0]
			o.Repo = repoToks[1]
			return nil
		}},
	{"POLL_FREQUENCY", "poll", "frequency with which to visit all issues in target GitHub repository", "15m", false,
		func(o *opts, value string) error {
			freq, err := time.ParseDuration(value)
			if err != nil {
				log.Fatalf("could not parse poll frequency '%s' as a valid duration", value)
			}
			if freq < 0 {
				log.Fatalln("poll frequency must be a positive duration")
			}
			o.PollFrequency = freq
			return nil
		}},
}

func parseOpts() (opts, error) {
	result := opts{}
	for _, schema := range optSchemas {
		var value string
		value, ok := os.LookupEnv(schema.EnvArgName)
		if ok {
			schema.OptSetter(&result, value)
			continue
		}
		flag.StringVar(&value, schema.FlagName, schema.DefaultValue, schema.FlagUsage)
		if value != "" {
			schema.OptSetter(&result, value)
			continue
		}
		if schema.DefaultValue == "" {
			if schema.Required {
				return opts{}, fmt.Errorf("no configured value for required option '%s'", schema.EnvArgName)
			}
			continue
		}
		schema.OptSetter(&result, schema.DefaultValue)
	}
	return result, nil
}
