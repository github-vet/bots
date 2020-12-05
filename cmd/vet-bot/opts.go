package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

type opts struct {
	GithubToken string
	IssuesFile  string
	ReposFile   string
	VisitedFile string
	TargetOwner string
	TargetRepo  string
	SingleOwner string
	SingleRepo  string
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
	{"TRACKING_FILE", "issues", "path to issues CSV file", "issues.csv", false,
		func(o *opts, value string) error { o.IssuesFile = value; return nil }},
	{"EXPERTS_FILE", "repos", "path to repos CSV file", "experts.csv", false,
		func(o *opts, value string) error { o.ReposFile = value; return nil }},
	{"GOPHERS_FILE", "visited", "path to visited repository CSV file", "gophers.csv", false,
		func(o *opts, value string) error { o.VisitedFile = value; return nil }},
	{"REPO_TO_READ", "single", "owner/repository of single repository to read", "", false,
		func(o *opts, value string) error {
			o.SingleOwner, o.SingleRepo = parseRepoString(value, "single")
			return nil
		}},
	{"GITHUB_REPO", "repo", "owner/repository of GitHub repo where issues will be filed", "", false,
		func(o *opts, value string) error {
			o.TargetOwner, o.TargetRepo = parseRepoString(value, "single")
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
		if schema.DefaultValue == "" && schema.Required {
			return opts{}, fmt.Errorf("no configured value for required option '%s'", schema.EnvArgName)
		}
		schema.OptSetter(&result, schema.DefaultValue)
	}
	return result, nil
}

func parseRepoString(str string, flag string) (string, string) {
	repoToks := strings.Split(str, "/")
	if len(repoToks) != 2 {
		log.Fatalf("could not parse %s flag '%s' which must be in owner/repository format", flag, str)
	}
	return repoToks[0], repoToks[1]
}
