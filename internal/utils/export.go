// Package utils holds helpers shared by more than one page of the UI.
package utils

import (
	"io"
	"os"

	"github.com/egoist/mygo"
)

// SaveCSV offers a native save dialog and streams the document to the
// chosen path, which Excel opens directly. The dialog opens and write runs
// on another goroutine, so a slow gather never blocks a frame, and a long
// document never sits whole in memory: write receives the file and writes
// it as it builds. done runs back on the UI thread with the outcome, and
// is skipped when the dialog is dismissed or unavailable.
func SaveCSV(title, defaultName string, write func(w io.Writer) error, done func(err error, path string)) {
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Title:       title,
			DefaultPath: defaultName,
			Filters:     []mygo.FileFilter{{Name: "CSV (Excel)", Extensions: []string{"csv"}}},
		})
		if path == "" {
			return
		}
		if err == nil {
			var f *os.File
			f, err = os.Create(path)
			if err == nil {
				err = write(f)
				if closeErr := f.Close(); err == nil {
					err = closeErr
				}
			}
		}
		mygo.RunOnMain(func() { done(err, path) })
	}()
}
