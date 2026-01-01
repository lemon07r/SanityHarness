package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/lemon07r/sanityharness/internal/runner"
	"github.com/lemon07r/sanityharness/internal/task"
	"github.com/lemon07r/sanityharness/tasks"
)

var (
	listLanguage string
	listJSON     bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available tasks",
	Long:  `Lists all available evaluation tasks, optionally filtered by language.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := runner.NewRunner(cfg, tasks.FS, tasksDir, logger)
		if err != nil {
			return err
		}
		defer r.Close()

		var taskList []*task.Task
		if listLanguage != "" {
			lang, err := task.ParseLanguage(listLanguage)
			if err != nil {
				return err
			}
			taskList, err = r.ListTasksByLanguage(lang)
			if err != nil {
				return err
			}
		} else {
			taskList, err = r.ListTasks()
			if err != nil {
				return err
			}
		}

		if listJSON {
			return outputJSON(taskList)
		}

		return outputTable(taskList)
	},
}

func init() {
	listCmd.Flags().StringVarP(&listLanguage, "language", "l", "", "filter by language (go, rust, typescript)")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "output as JSON")
}

func outputJSON(tasks []*task.Task) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(tasks)
}

func outputTable(taskList []*task.Task) error {
	if len(taskList) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SLUG\tLANGUAGE\tDIFFICULTY\tDESCRIPTION")
	fmt.Fprintln(w, "----\t--------\t----------\t-----------")

	for _, t := range taskList {
		desc := t.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Slug, t.Language, t.Difficulty, desc)
	}

	return w.Flush()
}
