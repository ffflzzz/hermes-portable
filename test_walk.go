package main

import (
	"fmt"
	"os"

	"github.com/lxn/walk"
)

func main() {
	var mw *walk.MainWindow
	var err error
	mw, err = walk.NewMainWindow()
	if err != nil {
		f, _ := os.Create("E:\\walk_debug.txt")
		f.WriteString("NewMainWindow error: " + err.Error())
		f.Close()
		return
	}
	fmt.Println("MainWindow created successfully")
	mw.SetVisible(true)
	mw.Run()
}
