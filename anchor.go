/*
 * Filename: /Users/bao/code/allhic/anchor.go
 * Path: /Users/bao/code/allhic
 * Created Date: Monday, June 4th 2018, 9:26:26 pm
 * Author: bao
 *
 * Copyright (c) 2018 Haibao Tang
 */

package allhic

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/biogo/hts/bam"
)

// Anchorer runs the merging algorithm
type Anchorer struct {
	Bamfile      string
	contigs      []*Contig
	nameToContig map[string]*Contig
	path         *Path
}

// Contig stores the name and length of each contig
type Contig struct {
	name        string
	length      int
	links       []*Link
	path        *Path
	start       int
	orientation int // 1 => +, -1 => -
	segments    []Range
}

// Link contains a specific inter-contig link
type Link struct {
	a, b       *Contig // Contig ids
	apos, bpos int     // Positions
}

// Path is a collection of ordered contigs
type Path struct {
	contigs      []*Contig // List of contigs
	LNode, RNode *Node     // Two nodes at each end
	length       int       // Cumulative length of all contigs
}

// Range tracks contig:start-end
type Range struct {
	start int
	end   int
	node  *Node
}

// SparseMatrix stores a big square matrix that is sparse
type SparseMatrix []map[int]int

// PathSet stores the set of paths
type PathSet map[*Path]bool

// Run kicks off the merging algorithm
func (r *Anchorer) Run() {
	var G Graph
	r.ExtractInterContigLinks()
	paths := r.makeTrivialPaths(r.contigs)
	prevPaths := len(paths)
	i := 0
	graphRemake := true
	for prevPaths > 1 {
		if graphRemake {
			i++
			log.Noticef("Starting iteration %d with %d paths", i, len(paths))
			G = r.makeGraph(paths)
		}
		CG := r.makeConfidenceGraph(G)
		paths = r.generatePathAndCycle(CG)
		if len(paths) == prevPaths {
			paths = r.removeSmallestPath(paths, G)
			graphRemake = true // Change this to false for speedup
		} else {
			graphRemake = true
		}
		prevPaths = len(paths)
	}

	// Test split the final path
	res, d := 500000, 4
	r.path = nil
	for path := range paths {
		if r.path == nil || path.length > r.path.length {
			r.path = path
		}
	}
	r.makeContigStarts()
	r.splitPath(res, d)
	log.Notice("Success")
}

// removeSmallestPath forces removal of the smallest path so that we can continue
// with the merging. This is also a small operation so we'll have to just modify
// the graph only slightly
func (r *Anchorer) removeSmallestPath(paths PathSet, G Graph) PathSet {
	var smallestPath *Path
	for path := range paths {
		if smallestPath == nil || path.length < smallestPath.length {
			smallestPath = path
		}
	}
	// Inactivate the nodes
	log.Noticef("Inactivate path %s (length=%d)", smallestPath, smallestPath.length)

	// Un-assign the contigs
	for _, contig := range smallestPath.contigs {
		contig.path = nil
	}

	// for _, node := range []*Node{smallestPath.LNode, smallestPath.RNode} {
	// 	if nb, ok := G[node]; ok {
	// 		for b := range nb {
	// 			delete(G[b], node)
	// 		}
	// 		delete(G, node)
	// 		fmt.Println("Deleted node", node)
	// 	}
	// }
	delete(paths, smallestPath)
	return paths
}

// printPaths shows the current details of the clustering
func printPaths(paths []*Path) {
	for _, path := range paths {
		fmt.Println(path)
	}
}

// makeTrivialPaths starts the initial construction of Path object, with one
// contig per Path (trivial Path)
func (r *Anchorer) makeTrivialPaths(contigs []*Contig) PathSet {
	// Initially make every contig a single Path object
	paths := PathSet{}
	for _, contig := range contigs {
		contig.orientation = 1
		path := &Path{
			contigs: []*Contig{contig},
		}
		contig.path = path
		path.bisect()
		paths[path] = true
	}

	return paths
}

// ExtractInterContigLinks extracts links from the Bamfile
func (r *Anchorer) ExtractInterContigLinks() {
	fh, _ := os.Open(r.Bamfile)
	prefix := RemoveExt(r.Bamfile)
	disfile := prefix + ".dis"
	idsfile := prefix + ".ids"

	log.Noticef("Parse bamfile `%s`", r.Bamfile)
	br, _ := bam.NewReader(fh, 0)
	defer br.Close()

	fdis, _ := os.Create(disfile)
	wdis := bufio.NewWriter(fdis)
	fids, _ := os.Create(idsfile)
	wids := bufio.NewWriter(fids)

	r.nameToContig = make(map[string]*Contig)
	refs := br.Header().Refs()
	for _, ref := range refs {
		contig := Contig{
			name:   ref.Name(),
			length: ref.Len(),
		}
		r.contigs = append(r.contigs, &contig)
		r.nameToContig[contig.name] = &contig
		fmt.Fprintf(wids, "%s\t%d\n", ref.Name(), ref.Len())
	}
	wids.Flush()
	log.Noticef("Extracted %d contigs to `%s`", len(r.contigs), idsfile)

	// Import links into pairs of contigs
	intraTotal, interTotal := 0, 0
	intraLinks := make(map[string][]int)
	for {
		rec, err := br.Read()
		if err != nil {
			if err != io.EOF {
				log.Error(err)
			}
			break
		}

		at, bt := rec.Ref.Name(), rec.MateRef.Name()
		a, b := r.nameToContig[at], r.nameToContig[bt]
		apos, bpos := rec.Pos, rec.MatePos

		// An intra-contig link
		if a == b {
			if link := abs(apos - bpos); link >= MinLinkDist {
				intraLinks[at] = append(intraLinks[at], link)
			}
			intraTotal++
			continue
		}

		// An inter-contig link
		a.links = append(a.links, &Link{
			a: a, b: b, apos: apos, bpos: bpos,
		})
		interTotal++
	}

	for _, contig := range r.contigs {
		sort.Slice(contig.links, func(i, j int) bool {
			return contig.links[i].apos < contig.links[j].apos
		})
	}

	// Write intra-links to .dis file
	for contig, links := range intraLinks {
		links = unique(links)
		fmt.Fprintf(wdis, "%s\t%s\n", contig, arrayToString(links, ","))
	}
	wdis.Flush()
	log.Noticef("Extracted %d intra-contig and %d inter-contig links",
		intraTotal, interTotal)
}

// reverse reverses the orientations of all components
func (r *Path) reverse() {
	c := r.contigs
	for i, j := 0, len(c)-1; i < j; i, j = i+1, j-1 {
		c[i], c[j] = c[j], c[i]
	}
	for _, contig := range c {
		contig.orientation = -contig.orientation
	}
}

// String prints the Path nicely
func (r *Path) String() string {
	tagContigs := make([]string, len(r.contigs))
	for i, contig := range r.contigs {
		tag := ""
		if contig.orientation < 0 {
			tag = "-"
		}
		tagContigs[i] = tag + contig.name
	}
	return strings.Join(tagContigs, " ")
}

// contigToNode takes as input contig and position, returns the nodeID
func contigToNode(contig *Contig, pos int) *Node {
	for _, rr := range contig.segments { // multiple 'segments'
		if pos >= rr.start && pos < rr.end {
			return rr.node
		}
	}
	log.Errorf("%s:%d not found", contig.name, pos)
	return nil
}

// linkToNodes takes as input link, returns two nodeIDs
func (r *Anchorer) linkToNodes(link *Link) (*Node, *Node) {
	a := contigToNode(link.a, link.apos)
	b := contigToNode(link.b, link.bpos)
	return a, b
}

// insertEdge adds just one link to the graph
func (r *Anchorer) insertEdge(G Graph, a, b *Node) {
	if _, aok := G[a]; aok {
		G[a][b] += 1.0
	} else {
		G[a] = map[*Node]float64{b: 1.0}
	}
}

// findMidPoint find the center of a path for bisect
func (r *Path) findMidPoint() (int, int) {
	r.length = 0
	for _, contig := range r.contigs {
		r.length += contig.length
	}

	midpos := r.length / 2
	cumsize := 0
	i := 0
	var contig *Contig
	for i, contig = range r.contigs {
		// midpos must be within this contig
		if cumsize+contig.length > midpos {
			break
		}
		cumsize += contig.length
	}
	contigpos := midpos - cumsize

	// --> ----> <-------- ------->
	//              | mid point here
	if contig.orientation == -1 {
		contigpos = contig.length - contigpos
	}
	return i, contigpos
}

// bisect cuts the Path into two parts
func (r *Path) bisect() {
	var contig *Contig
	i, contigpos := r.findMidPoint()
	contig = r.contigs[i]

	LNode := &Node{
		path: r,
	}
	RNode := &Node{
		path: r,
	}
	LNode.sister = RNode
	RNode.sister = LNode
	r.LNode = LNode
	r.RNode = RNode

	// Update the registry to convert contig:start-end range to nodes
	for k := 0; k < i; k++ { // Left contigs
		contig = r.contigs[k]
		contig.segments = []Range{
			Range{0, contig.length, LNode},
		}
	}

	// Handles the middle contig as a special case
	var leftRange, rightRange Range
	contig = r.contigs[i]
	if contig.orientation > 0 { // Forward orientation
		leftRange = Range{0, contigpos, LNode}
		rightRange = Range{contigpos, contig.length, RNode}
	} else { // Reverse orientation
		leftRange = Range{0, contigpos, RNode}
		rightRange = Range{contigpos, contig.length, LNode}
	}
	contig.segments = []Range{
		leftRange, rightRange,
	}

	for k := i + 1; k < len(r.contigs); k++ { // Right contigs
		contig = r.contigs[k]
		contig.segments = []Range{
			Range{0, contig.length, RNode},
		}
	}
}

// makeContigStarts returns starts of contigs within a path
func (r *Anchorer) makeContigStarts() {
	pos := 0
	for _, contig := range r.path.contigs {
		contig.start = pos
		pos += contig.length
	}
}

// findBin returns the i-th bin along the path
func findBin(contig *Contig, pos, resolution int) int {
	offset := pos
	if contig.orientation < 0 {
		offset = contig.length - pos
	}
	return (contig.start + offset) / resolution
}

// splitPath takes a path and look at joins that are weak
// Scans the path at certain resolution r, and search radius is d
func (r *Anchorer) splitPath(res, d int) {
	// Look at all intra-path links, then store the counts to a
	// sparse matrix, indexed by i, j, C[i, j] = # of links between
	// i-th locus and j-th locus
	bins := int(math.Ceil(float64(r.path.length) / float64(res)))
	log.Noticef("Contains %d bins at resolution %d bp", bins, res)
	// Initialize the sparse matrix
	C := make(SparseMatrix, bins)
	for i := 0; i < bins; i++ {
		C[i] = map[int]int{}
	}

	for _, contig := range r.path.contigs {
		for _, link := range contig.links {
			a := findBin(link.a, link.apos, res)
			b := findBin(link.b, link.bpos, res)
			if _, ok := C[a][b]; ok {
				C[a][b]++
			} else {
				C[a][b] = 1
			}

			if _, ok := C[b][a]; ok {
				C[b][a]++
			} else {
				C[b][a] = 1
			}
		}
	}

	breakPoints := printSparseMatrix(C, d)
	r.identifyGap(breakPoints, res)
}

// printMatrix shows all the entries in the matrix C that are higher than a certain
// cutoff, like 95-th percentile of all cells
func printSparseMatrix(C SparseMatrix, d int) []int {
	values := []int{}
	for a := range C {
		for _, val := range C[a] {
			values = append(values, val)
		}
	}

	sort.Ints(values)
	cutoff := values[len(values)*95/100]
	log.Noticef("Cutoff of cell value is at %d", cutoff)
	scores := []float64{}
	for a := range C {
		score := scoreTriangle(C, a, d, cutoff)
		scores = append(scores, score)
	}

	// Find the valley points
	breakPoints := []int{}
	for i := 0; i < len(scores)-2; i++ {
		if scores[i+1] <= scores[i] && scores[i+1] <= scores[i+2] && scores[i+1] < .1 {
			breakPoints = append(breakPoints, i+1)
		}
	}
	fmt.Println(breakPoints)
	log.Noticef("Found %d breakpoints", len(breakPoints))

	return breakPoints
}

// inspectGaps check each gap for the number of links <= 1Mb going across
func inspectGaps(path *Path) {
	// We need to quickly map all links to their [start, end] on the path
	// then increment all the link counts for each of the intervening gaps
	//
	// Here we use a data structure described in:
	// https://www.ncbi.nlm.nih.gov/pmc/articles/PMC3530906/
	// We store the starts and ends of links in sorted arrays
	// The `icount` algorithm then search an interval (or in this case) point
	// query into these sorted interval ends
	// BS := []int{}
	// BE := []int{}
}

// identifyGap prints out all the gaps that lie within the bin
func (r *Anchorer) identifyGap(breakPoints []int, res int) {
	contigStart := 0
	j := 0
	for i, contig := range r.path.contigs {
		if contigStart >= breakPoints[j]*res {
			if contigStart >= (breakPoints[j]+1)*res {
				for j < len(breakPoints) && contigStart >= (breakPoints[j]+1)*res {
					j++
				}
				// Exhausted, terminate
				if j == len(breakPoints) {
					break
				}
			} else {
				// We have a candidate
				fmt.Println(breakPoints[j], res, i, contigStart, contig.name)
				// fmt.Println(path.contigs[max(0, i-5):min(len(path.contigs)-1, i+5)])
			}
		}
		contigStart += contig.length
	}
}

// scoreTriangle sums up all the cells in the 1st quadrant that are d-distance
// away from the diagonal
func scoreTriangle(C SparseMatrix, a, d, cutoff int) float64 {
	expected := 0
	score := 0
	for i := a + 1; i < len(C); i++ {
		for j := a - 1; j >= 0 && i-j <= d; j-- {
			if _, ok := C[i][j]; ok {
				score += min(C[i][j], cutoff)
			}
			expected += cutoff
		}
	}
	fmt.Println(a, score, expected)

	// We are interested in finding all the misjoins, misjoins are dips in the
	// link coverage
	return float64(score) / float64(expected)
}
