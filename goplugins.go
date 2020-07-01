package main

import (
	"fmt"

	jar "github.com/vadikgo/goplugins/lib"
)

func main() {
	manifest, err := jar.ReadFile("kubernetes.hpi")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	} else {
		for key, element := range manifest {
			fmt.Println(key, "=>", element)
		}
	}
}
