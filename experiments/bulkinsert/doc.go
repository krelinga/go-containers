// Package bulkinsert measures whether a sorted map should offer bulk insertion.
//
// Adding k entries to a sorted slice of n by repeated Set is O(kn): every
// out-of-order insert memmoves half the backing array. Sorting the k additions
// once and merging two sorted runs is O(k log k + n + k). This finds where the
// second overtakes the first, and how much the sort inside it actually costs.
//
// Both variants here produce a new backing array, so the comparison is like for
// like. A real in-place SetAll would save the naive side its base clone, which
// shifts the crossover upward somewhat; see RESULTS.md.
//
// Run ./run.sh to regenerate bench.txt. See RESULTS.md for the findings and
// docs/adr/0004-bulk-insert.md for the decision they informed.
package bulkinsert
