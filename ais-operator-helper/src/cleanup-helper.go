package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func deleteConfigFiles(dirPath string) error {
    deletionFunc := func(path string, info os.FileInfo, err error) error {
        if err != nil {
            log.Printf("Error accessing path %q: %v", path, err)
            return err
        }
        if !info.IsDir() && strings.HasPrefix(filepath.Base(path), ".ais.") {
            // Delete file if it matches the pattern
            if err := os.Remove(path); err != nil {
                log.Printf("Error deleting file %q: %v", path, err)
                return err
            }
            log.Printf("Deleted file: %s", path)
        }
        return nil
    }

    // Walk through the directory and apply the deletion logic to each file
    if err := filepath.Walk(dirPath, deletionFunc); err != nil {
        return err
    }
    return nil
}

func main() {
    var dirPath string
    flag.StringVar(&dirPath, "dir", "/etc/ais/", "Directory path to delete files from")
    flag.Parse()

    log.SetFlags(log.LstdFlags | log.Lmicroseconds)

    // Delete matching files in the specified directory
    if err := deleteConfigFiles(dirPath); err != nil {
        log.Fatalf("Failed to delete files: %v", err)
    }

    log.Println("File deletion process completed successfully.")
}
