/**
 * Filename: /Users/bao/code/allhic/allhic/base.go
 * Path: /Users/bao/code/allhic/allhic
 * Created Date: Tuesday, January 2nd 2018, 8:07:22 pm
 * Author: bao
 *
 * Copyright (c) 2018 Haibao Tang
 */

package allhic

import (
	"fmt"
	"math"
	"os"
	"path"
	"sort"
	"strings"

	logging "github.com/op/go-logging"
)

const (
	// Version is the current version of ALLHIC
	Version = "0.8.2"
	// LB is lower bound for GoldenArray
	LB = 18
	// UB is upper bound for GoldenArray
	UB = 29
	// BB is span for GoldenArray
	BB = UB - LB + 1
	// PHI is natural log of golden ratio
	PHI = 0.4812118250596684 // math.Log(1.61803398875)
	// GRLB is the min item in GR
	GRLB = 5778
	// GRUB is the max item in GR
	GRUB = 1149851
	// OUTLIERTHRESHOLD is how many deviation from MAD
	OUTLIERTHRESHOLD = 3.5
	// MINSIZE is the minimum size cutoff for tig to be considered
	MINSIZE = 10000
	// MaxUint is the maximum possible value for uint type
	MaxUint = ^uint(0)
	// MinUint is the minimum possible value for uint type
	MinUint = 0
	// MaxInt is the maximum possible value for int type
	MaxInt = int(MaxUint >> 1)
	// MinInt is the minimum possible value for int type
	MinInt = -MaxInt - 1
	// GeometricBinSize is the max/min ratio for each bin
	GeometricBinSize = 1.0442737824274138403219664787399
	// MinLinkDist is the minimum link distance we care about
	MinLinkDist = 1 << 12
	// MaxLinkDist is the maximum link distance we care about
	MaxLinkDist = 1 << 28
	// BinNorm is a ratio to make the link density human readable
	BinNorm = 1000000.0
)

// GArray contains golden array of size BB
type GArray [BB]int

// GR is a precomputed list of exponents of golden ratio phi
var GR = [...]int{5778, 9349, 15127, 24476,
	39603, 64079, 103682, 167761,
	271443, 439204, 710647, 1149851}

var log = logging.MustGetLogger("allhic")
var format = logging.MustStringFormatter(
	`%{color}%{time:15:04:05} %{shortfunc} | %{level:.6s} %{color:reset} %{message}`,
)

// Backend is the default stderr output
var Backend = logging.NewLogBackend(os.Stderr, "", 0)

// BackendFormatter contains the fancy debug formatter
var BackendFormatter = logging.NewBackendFormatter(Backend, format)

// RemoveExt returns the substring minus the extension
func RemoveExt(filename string) string {
	return strings.TrimSuffix(filename, path.Ext(filename))
}

// IsNewerFile checks if file a is newer than file b
func IsNewerFile(a, b string) bool {
	af, aerr := os.Stat(a)
	bf, berr := os.Stat(b)
	if os.IsNotExist(aerr) || os.IsNotExist(berr) {
		return false
	}
	am := af.ModTime()
	bm := bf.ModTime()
	return am.Sub(bm) > 0
}

// Round makes a round number
func Round(input float64) float64 {
	if input < 0 {
		return math.Ceil(input - 0.5)
	}
	return math.Floor(input + 0.5)
}

// HmeanInt returns the harmonic mean
// That is:  n / (1/x1 + 1/x2 + ... + 1/xn)
func HmeanInt(a []int, amin, amax int) int {
	size := len(a)
	sum := 0.0
	for i := 0; i < size; i++ {
		val := a[i]
		if val > amax {
			val = amax
		} else if val < amin {
			val = amin
		}
		sum += 1.0 / float64(val)
	}
	return int(Round(float64(size) / sum))
}

// GoldenArray is given list of ints, we aggregate similar values so that it becomes an
// array of multiples of phi, where phi is the golden ratio.
//
// phi ^ 18 = 5778
// phi ^ 29 = 1149851
//
// So the array of counts go between 843 to 788196. One triva is that the
// exponents of phi gets closer to integers as N grows. See interesting
// discussion here:
// <https://www.johndcook.com/blog/2017/03/22/golden-powers-are-nearly-integers/>
func GoldenArray(a []int) (counts GArray) {
	for _, x := range a {
		c := int(Round(math.Log(float64(x)) / PHI))
		if c < LB {
			c = LB
		} else if c > UB {
			c = UB
		}
		counts[c-LB]++
	}
	return
}

// abs gets the absolute value of an int
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// min gets the minimum for two ints
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// min gets the maximum for two ints
func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// sum gets the sum for an int slice
func sum(a []int) int {
	ans := 0
	for _, x := range a {
		ans += x
	}
	return ans
}

// sumf gets the sum for an int slice
func sumf(a []float64) float64 {
	ans := 0.0
	for _, x := range a {
		ans += x
	}
	return ans
}

// arrayToString print comma-separated int slice
func arrayToString(a []int, delim string) string {
	return strings.Trim(strings.Replace(fmt.Sprint(a), " ", delim, -1), "[]")
}

// median gets the median value of an array
func median(s []float64) float64 {
	// Make a sorted copy
	numbers := make([]float64, len(s))
	copy(numbers, s)
	sort.Float64s(numbers)

	middle := len(numbers) / 2
	result := numbers[middle]
	if len(numbers)%2 == 0 {
		result = (result + numbers[middle-1]) / 2
	}
	return result
}

// OutlierCutoff implements Iglewicz and Hoaglin's robust, returns the cutoff values -
// lower bound and upper bound.
func OutlierCutoff(a []float64) (float64, float64) {
	M := median(a)
	D := make([]float64, len(a))
	for i := 0; i < len(a); i++ {
		D[i] = math.Abs(a[i] - M)
	}
	MAD := median(D)
	C := OUTLIERTHRESHOLD / .67449 * MAD
	return M - C, M + C
}

// Make2DSlice allocates a 2D matrix with shape (m, n)
func Make2DSlice(m, n int) [][]int {
	P := make([][]int, m)
	for i := 0; i < m; i++ {
		P[i] = make([]int, n)
	}
	return P
}

// Make2DGArraySlice allocates a 2D matrix with shape (m, n)
func Make2DGArraySlice(m, n int) [][]GArray {
	P := make([][]GArray, m)
	for i := 0; i < m; i++ {
		P[i] = make([]GArray, n)
	}
	return P
}

// Make3DSlice allocates a 3D matrix with shape (m, n, o)
func Make3DSlice(m, n, o int) [][][]int {
	P := make([][][]int, m)
	for i := 0; i < m; i++ {
		P[i] = make([][]int, n)
		for j := 0; j < n; j++ {
			P[i][j] = make([]int, o)
		}
	}
	return P
}
