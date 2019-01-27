// Copyright (c) 2019, The Emergent Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package etensor

// Shape manages a tensor's shape information, including strides and dimension names
// and can compute the flat index into an underlying 1D data storage array based on an
// n-dimensional index (and vice-versa).
// This is fully compatible with (and largely taken from) apache/arrow tensors.
// except that we use plain int instead of int64, because on all relevant platforms
// int is *already* 64 and using plain int is much easier.
// Per C / Go / Python conventions (and unlike emergent) indexes are ordered from
// outer to inner left-to-right, so the inner-most is right-most.
// This is called Row-Major order, and is the default.
// It is also possible to use Column-Major order, which is used in R, Julia, and MATLAB,
// and emergent, where the inner-most index is first and outer-most last.
// In principle, you can organize memory independent of the conceptual order of indexes
// but for efficiency it is best to organize memory in the way that indexes are accessed.
type Shape struct {
	shape   []int
	strides []int
	names   []string
}

// NewShape returns a new shape object initialized with params.
// If strides is nil, row-major strides will be inferred.
// If names is nil, a slice of empty strings will be created.
func NewShape(shape, strides []int, names []string) *Shape {
	sh := &Shape{}
	sh.SetShape(shape, strides, names)
	return sh
}

// SetShape sets the shape parameters.
// If strides is nil, row-major strides will be inferred.
// If names is nil, a slice of empty strings will be created.
func (sh *Shape) SetShape(shape, strides []int, names []string) {
	sh.shape = CopyInts(shape)
	if strides == nil {
		sh.strides = RowMajorStrides(shape)
	} else {
		sh.strides = CopyInts(strides)
	}
	if names == nil {
		sh.names = make([]string, len(sh.shape))
	} else {
		sh.names = CopyStrings(names)
	}
}

// CopyShape copies the shape parameters from another Shape struct.
// copies the data so it is not accidentally subject to updates.
func (sh *Shape) CopyShape(cp *Shape) {
	sh.shape = CopyInts(cp.shape)
	sh.strides = CopyInts(cp.strides)
	sh.names = CopyStrings(cp.names)
}

// AddShapes returns a new shape by adding two shapes one after the other.
// uses Row / Col order of the first shape for resulting shape
func AddShapes(shape1, shape2 *Shape) *Shape {
	sh1 := shape1.Shape()
	sh2 := shape2.Shape()
	nsh := make([]int, len(sh1)+len(sh2))
	copy(nsh, sh1)
	copy(nsh[len(sh1):], sh2)
	rm := shape1.IsRowMajor()
	var nstr []int
	if rm {
		nstr = RowMajorStrides(nsh)
	} else {
		nstr = ColMajorStrides(nsh)
	}
	nms := make([]string, len(sh1)+len(sh2))
	copy(nms, shape1.DimNames())
	copy(nms[len(sh1):], shape2.DimNames())
	return NewShape(nsh, nstr, nms)
}

// Len returns the total length of elements in the tensor (i.e., the product of
// the shape sizes)
func (sh *Shape) Len() int {
	o := int(1)
	for _, v := range sh.shape {
		o *= v
	}
	return int(o)
}

// IntTo64 converts an []int slice to an []int64 slice
func (sh *Shape) IntTo64(isl []int) []int64 {
	is := make([]int64, len(isl))
	for i := range isl {
		is[i] = int64(isl[i])
	}
	return is
}

func (sh *Shape) Shape() []int       { return sh.shape }
func (sh *Shape) Strides() []int     { return sh.strides }
func (sh *Shape) Shape64() []int64   { return sh.IntTo64(sh.shape) }
func (sh *Shape) Strides64() []int64 { return sh.IntTo64(sh.strides) }

func (sh *Shape) DimNames() []string   { return sh.names }
func (sh *Shape) NumDims() int         { return len(sh.shape) }
func (sh *Shape) DimName(i int) string { return sh.names[i] }

func (sh *Shape) IsContiguous() bool {
	return sh.IsRowMajor() || sh.IsColMajor()
}

func (sh *Shape) IsRowMajor() bool {
	strides := RowMajorStrides(sh.shape)
	return EqualInts(strides, sh.strides)
}

func (sh *Shape) IsColMajor() bool {
	strides := ColMajorStrides(sh.shape)
	return EqualInts(strides, sh.strides)
}

// Offset returns the "flat" 1D array index into an element at the given n-dimensional index
// No checking is done on the length of the index relative to the shape of the tensor.
func (sh *Shape) Offset(index []int) int {
	var offset int
	for i, v := range index {
		offset += v * sh.strides[i]
	}
	return offset
}

// Index returns the n-dimensional index from a "flat" 1D array index
// No checking is done on the length of the index relative to the shape of the tensor.
// func (sh *Shape) Index(offset int) []int {
// 	index := make([]int, len(sh.strides))
// 	for i, v := range sh.strides {
// 		index[i] = sh.strides[i]
// 	}
// 	return index
// }

// RowMajorStrides returns strides for shape where the first dimension is outer-most
// and subsequent dimensions are progressively inner
func RowMajorStrides(shape []int) []int {
	rem := int(1)
	for _, v := range shape {
		rem *= v
	}

	if rem == 0 {
		strides := make([]int, len(shape))
		rem := int(1)
		for i := range strides {
			strides[i] = rem
		}
		return strides
	}

	strides := make([]int, len(shape))
	for i, v := range shape {
		rem /= v
		strides[i] = rem
	}
	return strides
}

// ColMajorStrides returns strides for shape where the first dimension is inner-most
// and subsequent dimensions are progressively outer
func ColMajorStrides(shape []int) []int {
	total := int(1)
	for _, v := range shape {
		if v == 0 {
			strides := make([]int, len(shape))
			for i := range strides {
				strides[i] = total
			}
			return strides
		}
	}

	strides := make([]int, len(shape))
	for i, v := range shape {
		strides[i] = total
		total *= v
	}
	return strides
}

// EqualInts compares two int slices and returns true if they are equal
func EqualInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// CopyInts makes a copy of an int slice
func CopyInts(a []int) []int {
	ns := make([]int, len(a))
	copy(ns, a)
	return ns
}

// CopyStrings makes a copy of a string slice
func CopyStrings(a []string) []string {
	ns := make([]string, len(a))
	copy(ns, a)
	return ns
}