package pio

import (
	"fmt"
)

func Example() {
	vec := [][]byte{{1, 2, 3}, {4, 5, 6, 7, 8, 9}, {10, 11, 12, 13}}
	fmt.Println(VecLen(vec))

	vec = VecSlice(vec, 1, -1)
	fmt.Println(vec)

	vec = VecSlice(vec, 2, -1)
	fmt.Println(vec)

	vec = VecSlice(vec, 8, 8)
	fmt.Println(vec)

	// Output:
	// 13
	// [[2 3] [4 5 6 7 8 9] [10 11 12 13]]
	// [[4 5 6 7 8 9] [10 11 12 13]]
	// []
}
