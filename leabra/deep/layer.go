// Copyright (c) 2019, The Emergent Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package deep

import (
	"fmt"

	"github.com/chewxy/math32"
	"github.com/emer/emergent/emer"
	"github.com/emer/emergent/leabra/leabra"
	"github.com/goki/ki/kit"
)

// deep.Layer is the DeepLeabra layer, based on basic rate-coded leabra.Layer
type Layer struct {
	leabra.Layer
	DeepBurst DeepBurstParams `desc:"parameters for computing DeepBurst from act, in Superficial layers (but also needed in Deep layers for deep self connections)"`
	DeepCtxt  DeepCtxtParams  `desc:"parameters for computing DeepCtxt in Deep layers, from BurstCtxt inputs from Super senders"`
	DeepTRC   DeepTRCParams   `desc:"parameters for computing TRC plus-phase (outcome) activations based on TRCBurstGe excitatory input from BurstTRC projections"`
	DeepAttn  DeepAttnParams  `desc:"parameters for computing DeepAttn and DeepLrn attentional modulation signals based on DeepAttn projection inputs integrated into AttnGe excitatory conductances"`
	DeepNeurs []Neuron        `desc:"slice of extra deep.Neuron state for this layer -- flat list of len = Shape.Len(). You must iterate over index and use pointer to modify values."`
}

// AsLeabra returns this layer as a leabra.Layer -- all derived layers must redefine
// this to return the base Layer type, so that the LeabraLayer interface does not
// need to include accessors to all the basic stuff
func (ly *Layer) AsLeabra() *leabra.Layer {
	return &ly.Layer
}

func (ly *Layer) Defaults() {
	ly.Act.Defaults()
	ly.Inhib.Defaults()
	ly.Learn.Defaults()
	ly.Inhib.Layer.On = true
	for _, pj := range ly.RecvPrjns {
		pj.Defaults()
	}
}

// UpdateParams updates all params given any changes that might have been made to individual values
// including those in the receiving projections of this layer
func (ly *Layer) UpdateParams() {
	ly.Act.Update()
	ly.Inhib.Update()
	ly.Learn.Update()
	for _, pj := range ly.RecvPrjns {
		pj.UpdateParams()
	}
}

// SetParams sets given parameters to this layer, if the target type is Layer
// calls UpdateParams to ensure derived parameters are all updated.
// If setMsg is true, then a message is printed to confirm each parameter that is set.
// it always prints a message if a parameter fails to be set.
func (ly *Layer) SetParams(pars emer.Params, setMsg bool) bool {
	trg := pars.Target()
	if trg != "Layer" {
		return false
	}
	pars.Set(ly, setMsg)
	ly.UpdateParams()
	return true
}

// UnitVarNames returns a list of variable names available on the units in this layer
func (ly *Layer) UnitVarNames() []string {
	return AllNeuronVars
}

// UnitVals is emer.Layer interface method to return values of given variable
func (ly *Layer) UnitVals(varNm string) ([]float32, error) {
	vidx, err := leabra.NeuronVarByName(varNm)
	if err == nil {
		return ly.Layer.UnitVals(varNm)
	}
	vidx, err = NeuronVarByName(varNm)
	if err != nil {
		return nil, err
	}
	vs := make([]float32, len(ly.DeepNeurs))
	for i := range ly.DeepNeurs {
		nrn := &ly.DeepNeurs[i]
		vs[i] = nrn.VarByIndex(vidx)
	}
	return vs, nil
}

// UnitVal returns value of given variable name on given unit,
// using shape-based dimensional index
func (ly *Layer) UnitVal(varNm string, idx []int) (float32, error) {
	_, err := leabra.NeuronVarByName(varNm)
	if err == nil {
		return ly.Layer.UnitVal(varNm, idx)
	}
	fidx := ly.Shape.Offset(idx)
	nn := len(ly.DeepNeurs)
	if fidx < 0 || fidx >= nn {
		return 0, fmt.Errorf("Layer UnitVal index: %v out of range, N = %v", fidx, nn)
	}
	nrn := &ly.DeepNeurs[fidx]
	return nrn.VarByName(varNm)
}

// UnitVal1D returns value of given variable name on given unit,
// using 1-dimensional index.
func (ly *Layer) UnitVal1D(varNm string, idx int) (float32, error) {
	_, err := leabra.NeuronVarByName(varNm)
	if err == nil {
		return ly.Layer.UnitVal1D(varNm, idx)
	}
	nn := len(ly.DeepNeurs)
	if idx < 0 || idx >= nn {
		return 0, fmt.Errorf("Layer UnitVal1D index: %v out of range, N = %v", idx, nn)
	}
	nrn := &ly.DeepNeurs[idx]
	return nrn.VarByName(varNm)
}

// Build constructs the layer state, including calling Build on the projections
// you MUST have properly configured the Inhib.Pool.On setting by this point
// to properly allocate Pools for the unit groups if necessary.
func (ly *Layer) Build() error {
	err := ly.Layer.Build()
	if err != nil {
		return err
	}
	nu := ly.Shape.Len()
	ly.DeepNeurs = make([]Neuron, nu)
	return nil
}

//////////////////////////////////////////////////////////////////////////////////////
//  Init methods

func (ly *Layer) InitActs() {
	ly.Layer.InitActs()
	for ni := range ly.DeepNeurs {
		nrn := &ly.DeepNeurs[ni]
		nrn.ActNoAttn = 0
		nrn.DeepBurst = 0
		nrn.DeepBurstPrv = 0
		nrn.DeepCtxt = 0
		nrn.TRCBurstGe = 0
		nrn.DeepBurstSent = 0
		nrn.AttnGe = 0
		nrn.DeepAttn = 0
		nrn.DeepLrn = 0
	}
}

func (ly *Layer) DecayState(decay float32) {
	ly.Layer.DecayState(decay)
	for ni := range ly.DeepNeurs {
		nrn := &ly.DeepNeurs[ni]
		nrn.DeepBurstSent = 0
	}
}

// SendGeDelta sends change in activation since last sent, if above thresholds.
// Deep version sends either to standard Ge or AttnGe for DeepAttn projections.
func (ly *Layer) SendGeDelta() {
	for ni := range ly.Neurons {
		nrn := &ly.Neurons[ni]
		if nrn.Act > ly.Act.OptThresh.Send {
			delta := nrn.Act - nrn.ActSent
			if math32.Abs(delta) > ly.Act.OptThresh.Delta {
				for si := range ly.SendPrjns {
					sp := ly.SendPrjns[si]
					if sp.IsOff() {
						continue
					}
					pj := sp.(*Prjn)
					if pj.Type == BurstCtxt || pj.Type == BurstTRC {
						continue
					}
					if pj.Type == DeepAttn {
						pj.SendAttnGeDelta(ni, delta)
					} else {
						pj.SendGeDelta(ni, delta)
					}
				}
				nrn.ActSent = nrn.Act
			}
		} else if nrn.ActSent > ly.Act.OptThresh.Send {
			delta := -nrn.ActSent // un-send the last above-threshold activation to get back to 0
			for si := range ly.SendPrjns {
				sp := ly.SendPrjns[si]
				if sp.IsOff() {
					continue
				}
				pj := sp.(*Prjn)
				if pj.Type == BurstCtxt || pj.Type == BurstTRC {
					continue
				}
				if pj.Type == DeepAttn {
					pj.SendAttnGeDelta(ni, delta)
				} else {
					pj.SendGeDelta(ni, delta)
				}
			}
			nrn.ActSent = 0
		}
	}
}

// SendTRCBurstGeDelta sends change in DeepBurst activation since last sent, over BurstTRC
// projections.
func (ly *Layer) SendTRCBurstGeDelta() {
	for ni := range ly.DeepNeurs {
		nrn := &ly.DeepNeurs[ni]
		if nrn.DeepBurst > ly.Act.OptThresh.Send {
			delta := nrn.DeepBurst - nrn.DeepBurstSent
			if math32.Abs(delta) > ly.Act.OptThresh.Delta {
				for si := range ly.SendPrjns {
					sp := ly.SendPrjns[si]
					if sp.IsOff() {
						continue
					}
					pj := sp.(*Prjn)
					if pj.Type != BurstTRC {
						continue
					}
					pj.SendTRCBurstGeDelta(ni, delta)
				}
				nrn.DeepBurstSent = nrn.DeepBurst
			}
		} else if nrn.DeepBurstSent > ly.Act.OptThresh.Send {
			delta := -nrn.DeepBurstSent // un-send the last above-threshold activation to get back to 0
			for si := range ly.SendPrjns {
				sp := ly.SendPrjns[si]
				if sp.IsOff() {
					continue
				}
				pj := sp.(*Prjn)
				if pj.Type != BurstTRC {
					continue
				}
				pj.SendTRCBurstGeDelta(ni, delta)
			}
			nrn.DeepBurstSent = 0
		}
	}
}

//////////////////////////////////////////////////////////////////////////////////////
//  LayerType

// DeepLeabra extensions to the emer.LayerType types

//go:generate stringer -type=LayerType

var KiT_LayerType = kit.Enums.AddEnum(LayerTypeN, false, nil)

// The DeepLeabra layer types
const (
	// Super are superficial-layer neurons, which also compute DeepBurst activation as a
	// thresholded version of superficial activation, and send that to both TRC (for plus
	// phase outcome) and Deep layers (for DeepCtxt temporal context).
	Super emer.LayerType = emer.LayerTypeN + iota

	// Deep are deep-layer neurons, reflecting activation of layer 6 regular spiking
	// CT corticothalamic neurons, which drive both attention in Super (via DeepAttn
	// projections) and  predictions in TRC (Pulvinar) via standard projections.
	Deep

	// TRC are thalamic relay cell neurons, typically in the Pulvinar, which alternately reflect
	// predictions driven by Deep layer projections, and actual outcomes driven by BurstTRC
	// projections from corresponding Super layer neurons that provide strong driving inputs to
	// TRC neurons.
	TRC

	LayerTypeN
)
