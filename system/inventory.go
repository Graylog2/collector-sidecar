package system

import "runtime"

type Inventory struct {
}

func NewInventory() *Inventory {
	return &Inventory{}
}

func (inv *Inventory) Linux() bool {
	return runtime.GOOS == "linux"
}

func (inv *Inventory) Darwin() bool {
	return runtime.GOOS == "darwin"
}

func (inv *Inventory) Windows() bool {
	return runtime.GOOS == "windows"
}
