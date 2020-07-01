package main

import (
	"fmt"
	"github.com/vadikgo/goplugins/lib"
)

func main() {
	manifest, err := ReadFile("kubernetes.hpi")
	fmt.Printf("Error: %v\n", err)
	fmt.Printf("Specification-Vendor: %s\n", manifest["Specification-Vendor"])
	fmt.Printf("Implementation-Vendor-Id: %s\n", manifest["Implementation-Vendor-Id"])
	fmt.Printf("Specification-Version: %s\n", manifest["Specification-Version"])
}
