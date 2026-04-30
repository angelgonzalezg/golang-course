// Switch Statements
// Topic: switch selects one branch from multiple cases.
// Args/Input: it can check a value, conditions, or the runtime type of interface{}.
// Usage: replace long if/else chains with cleaner branching logic.
// Key idea: switch improves readability when checking many related cases.
package main

import (
	"fmt"
	"time"
)

func main() {
	day := "Sunday"
	fmt.Printf("Today is %s\n", day)

	switch day { // Value switch
	case "Saturday", "Sunday":
		fmt.Println("Weekend! No work")
	case "Monday", "Tuesday":
		fmt.Println("Work days. Lots of meetings")
	default:
		fmt.Println("Mid-week")
	}

	switch hour := time.Now().Hour(); { // Condition switch
	case hour < 12:
		fmt.Println("Good morning")
	case hour < 17:
		fmt.Println("Good afternoon")
	default:
		fmt.Println("Good evening")
	}

	checkType := func(i interface{}) {
		switch v := i.(type) { // Type switch
		case int:
			fmt.Printf("Integer: %d\n", v)
		case string:
			fmt.Printf("String: %s\n", v)
		case bool:
			fmt.Printf("Boolean: %t\n", v)
		default:
			fmt.Printf("Unknown type: %T\n", v)
		}
	}

	checkType(21)
	checkType("Test")
	checkType(true)
	checkType(3.213)

}
