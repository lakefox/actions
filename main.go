package main

import (
	"server/src/actions"
)

func main() {
	a := actions.Action("./Nginx")
	a.Run("install")
}
