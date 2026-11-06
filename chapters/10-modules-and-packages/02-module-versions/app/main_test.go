package main

// This is an integration check of both required module paths and their public APIs.
func Example_moduleVersions() {
	main()
	// Output:
	// == 1. Version 1 returns a string ==
	// Hello, Ana!
	// blank name: Hello, friend!
	//
	// == 2. Version 2 returns a string and an error ==
	// Hello, Ana!
	// blank name rejected: true
}
