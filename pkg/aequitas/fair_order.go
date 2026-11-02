// Copyright (c) 2026 ZengLLQ
// Licensed under the MIT License

package aequitas

import (
	"sort"

	"github.com/ZengLLQ/zenith-sequencer/pkg/types"
)

type Hash = types.Hash

// NodeReport contains the local arrival sequence of transactions at a validator node
type NodeReport struct {
	NodeID       string
	ArrivalOrder []Hash
}

// FairOrderEngine constructs a fair total order of transactions using the Aequitas algorithm
type FairOrderEngine struct {
	fairnessThreshold float64 // gamma in (0.5, 1.0]
}

func NewFairOrderEngine(threshold float64) *FairOrderEngine {
	if threshold <= 0.5 || threshold > 1.0 {
		threshold = 0.67 // Default 2/3 BFT threshold
	}
	return &FairOrderEngine{
		fairnessThreshold: threshold,
	}
}

// ComputeFairOrder determines the fair sequence from validator arrival observations
func (f *FairOrderEngine) ComputeFairOrder(reports []NodeReport) []Hash {
	if len(reports) == 0 {
		return nil
	}

	// 1. Gather all unique transaction hashes
	txSet := make(map[Hash]bool)
	for _, rep := range reports {
		for _, tx := range rep.ArrivalOrder {
			txSet[tx] = true
		}
	}

	allTxs := make([]Hash, 0, len(txSet))
	for tx := range txSet {
		allTxs = append(allTxs, tx)
	}

	if len(allTxs) <= 1 {
		return allTxs
	}

	// 2. Build pairwise precedence frequency: count how many nodes saw txA before txB
	totalNodes := float64(len(reports))
	requiredVotes := int(f.fairnessThreshold*totalNodes + 0.0001)
	if requiredVotes < 1 {
		requiredVotes = 1
	}

	precedesCount := make(map[Hash]map[Hash]int)
	for _, txA := range allTxs {
		precedesCount[txA] = make(map[Hash]int)
	}

	for _, rep := range reports {
		pos := make(map[Hash]int, len(rep.ArrivalOrder))
		for i, tx := range rep.ArrivalOrder {
			pos[tx] = i
		}

		for i := 0; i < len(allTxs); i++ {
			txA := allTxs[i]
			posA, okA := pos[txA]
			for j := i + 1; j < len(allTxs); j++ {
				txB := allTxs[j]
				posB, okB := pos[txB]

				if okA && okB {
					if posA < posB {
						precedesCount[txA][txB]++
					} else if posB < posA {
						precedesCount[txB][txA]++
					}
				} else if okA && !okB {
					precedesCount[txA][txB]++
				} else if !okA && okB {
					precedesCount[txB][txA]++
				}
			}
		}
	}

	// 3. Construct directed dependency graph: edge txA -> txB if count(txA before txB) >= requiredVotes
	adj := make(map[Hash][]Hash)
	for _, txA := range allTxs {
		for _, txB := range allTxs {
			if txA == txB {
				continue
			}
			if precedesCount[txA][txB] >= requiredVotes {
				adj[txA] = append(adj[txA], txB)
			}
		}
	}

	// 4. Tarjan's Strongly Connected Components (SCC) to resolve dependency cycles
	sccs := tarjanSCC(allTxs, adj)

	// 5. Build Condensation DAG of SCCs
	sccMap := make(map[Hash]int)
	for sccIdx, comp := range sccs {
		for _, tx := range comp {
			sccMap[tx] = sccIdx
		}
	}

	sccAdj := make(map[int]map[int]bool)
	inDegree := make([]int, len(sccs))
	for i := range sccs {
		sccAdj[i] = make(map[int]bool)
	}

	for _, txA := range allTxs {
		sccA := sccMap[txA]
		for _, txB := range adj[txA] {
			sccB := sccMap[txB]
			if sccA != sccB && !sccAdj[sccA][sccB] {
				sccAdj[sccA][sccB] = true
				inDegree[sccB]++
			}
		}
	}

	// 6. Topological Sort of SCC condensation graph (Kahn's algorithm)
	queue := make([]int, 0)
	for i := range sccs {
		if inDegree[i] == 0 {
			queue = append(queue, i)
		}
	}

	sortedSCCs := make([]int, 0, len(sccs))
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		sortedSCCs = append(sortedSCCs, curr)

		for next := range sccAdj[curr] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	// In case of unvisited due to isolated components, append remaining
	if len(sortedSCCs) < len(sccs) {
		visited := make(map[int]bool)
		for _, s := range sortedSCCs {
			visited[s] = true
		}
		for i := range sccs {
			if !visited[i] {
				sortedSCCs = append(sortedSCCs, i)
			}
		}
	}

	// 7. Flatten SCCs into final total order. Inside each SCC, break ties deterministically by hash
	finalOrder := make([]Hash, 0, len(allTxs))
	for _, sccIdx := range sortedSCCs {
		comp := sccs[sccIdx]
		sort.Slice(comp, func(i, j int) bool {
			return comp[i].String() < comp[j].String()
		})
		finalOrder = append(finalOrder, comp...)
	}

	return finalOrder
}

// tarjanSCC finds all strongly connected components in the graph
func tarjanSCC(nodes []Hash, adj map[Hash][]Hash) [][]Hash {
	index := 0
	indices := make(map[Hash]int)
	lowlink := make(map[Hash]int)
	onStack := make(map[Hash]bool)
	stack := make([]Hash, 0)
	sccs := make([][]Hash, 0)

	var strongConnect func(v Hash)
	strongConnect = func(v Hash) {
		indices[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, visited := indices[w]; !visited {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] {
				if indices[w] < lowlink[v] {
					lowlink[v] = indices[w]
				}
			}
		}

		if lowlink[v] == indices[v] {
			comp := make([]Hash, 0)
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			sccs = append(sccs, comp)
		}
	}

	for _, v := range nodes {
		if _, visited := indices[v]; !visited {
			strongConnect(v)
		}
	}

	return sccs
}
