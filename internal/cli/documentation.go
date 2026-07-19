package cli

import "github.com/spf13/cobra"

const (
	documentationEffectAnnotation       = "libredash.dev/effect"
	documentationConfirmationAnnotation = "libredash.dev/confirmation"
)

type commandSafety struct {
	effect       string
	confirmation string
}

func annotateCommandDocumentation(root *cobra.Command) {
	var visit func(*cobra.Command)
	visit = func(command *cobra.Command) {
		if safety, ok := documentedCommandSafety[command.CommandPath()]; ok {
			if command.Annotations == nil {
				command.Annotations = map[string]string{}
			}
			command.Annotations[documentationEffectAnnotation] = safety.effect
			command.Annotations[documentationConfirmationAnnotation] = safety.confirmation
		}
		for _, child := range command.Commands() {
			visit(child)
		}
	}
	visit(root)
}

var documentedCommandSafety = map[string]commandSafety{
	"libredash":                                 {effect: "local-write", confirmation: "never"},
	"libredash admin backup":                    {effect: "local-write", confirmation: "never"},
	"libredash admin initialize":                {effect: "write", confirmation: "never"},
	"libredash admin maintenance":               {effect: "destructive", confirmation: "conditional"},
	"libredash admin restore":                   {effect: "destructive", confirmation: "required"},
	"libredash admin storage cleanup":           {effect: "destructive", confirmation: "conditional"},
	"libredash agent ask":                       {effect: "write", confirmation: "never"},
	"libredash agent conversations":             {effect: "read", confirmation: "never"},
	"libredash agent tools":                     {effect: "read", confirmation: "never"},
	"libredash api call":                        {effect: "dynamic", confirmation: "never"},
	"libredash api describe":                    {effect: "read", confirmation: "never"},
	"libredash api list":                        {effect: "read", confirmation: "never"},
	"libredash config validate":                 {effect: "read", confirmation: "never"},
	"libredash dashboards describe":             {effect: "read", confirmation: "never"},
	"libredash dashboards filter-options":       {effect: "read", confirmation: "never"},
	"libredash dashboards list":                 {effect: "read", confirmation: "never"},
	"libredash dashboards page":                 {effect: "read", confirmation: "never"},
	"libredash dashboards query-page":           {effect: "read", confirmation: "never"},
	"libredash dashboards table-data":           {effect: "read", confirmation: "never"},
	"libredash dashboards visual":               {effect: "read", confirmation: "never"},
	"libredash dashboards visual-data":          {effect: "read", confirmation: "never"},
	"libredash data plan":                       {effect: "read", confirmation: "never"},
	"libredash data revisions current":          {effect: "read", confirmation: "never"},
	"libredash data revisions list":             {effect: "read", confirmation: "never"},
	"libredash data sync":                       {effect: "write", confirmation: "never"},
	"libredash deploy":                          {effect: "write", confirmation: "conditional"},
	"libredash healthcheck":                     {effect: "read", confirmation: "never"},
	"libredash login":                           {effect: "local-write", confirmation: "never"},
	"libredash plan":                            {effect: "read", confirmation: "never"},
	"libredash schema export":                   {effect: "local-write", confirmation: "never"},
	"libredash search":                          {effect: "read", confirmation: "never"},
	"libredash semantic-models dataset":         {effect: "read", confirmation: "never"},
	"libredash semantic-models datasets":        {effect: "read", confirmation: "never"},
	"libredash semantic-models describe":        {effect: "read", confirmation: "never"},
	"libredash semantic-models explain-preview": {effect: "read", confirmation: "never"},
	"libredash semantic-models explain-query":   {effect: "read", confirmation: "never"},
	"libredash semantic-models fields":          {effect: "read", confirmation: "never"},
	"libredash semantic-models list":            {effect: "read", confirmation: "never"},
	"libredash semantic-models preview":         {effect: "read", confirmation: "never"},
	"libredash semantic-models query":           {effect: "read", confirmation: "never"},
	"libredash serve":                           {effect: "local-write", confirmation: "never"},
	"libredash validate":                        {effect: "read", confirmation: "never"},
	"libredash workspaces list":                 {effect: "read", confirmation: "never"},
}
