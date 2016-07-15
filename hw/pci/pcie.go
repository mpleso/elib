package pci

type PCIECapabilityHeader struct {
	CapabilityHeader
	Capabilities    uint16
	Dev, Link, Slot struct {
		Capabilities uint32
		Control      uint16
		Status       uint16
	}
	Root struct {
		Control      uint16
		Capabilities uint16
		Status       uint32
	}
}
