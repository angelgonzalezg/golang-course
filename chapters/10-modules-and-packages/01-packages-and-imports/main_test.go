package main

func Example_packagesAndImports() {
	main()
	// Output:
	// == 1. Use an exported type and method ==
	// Hello, Ana María!
	//
	// == 2. Initialize through the package API ==
	// Hola, Luis!
	// blank name: true
	//
	// == 3. Inspect an exported error across a package boundary ==
	// unsupported language: true
}
