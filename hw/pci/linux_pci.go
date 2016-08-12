package pci

// Linux PCI code

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
)

var sysBusPciPath string = "/sys/bus/pci/devices"

func (d *Device) SysfsPath(format string, args ...interface{}) (path string) {
	path = filepath.Join(sysBusPciPath, d.Addr.String(), fmt.Sprintf(format, args...))
	return
}

func (d *Device) SysfsOpenFile(format string, mode int, args ...interface{}) (f *os.File, err error) {
	fn := d.SysfsPath(format, args...)
	f, err = os.OpenFile(fn, mode, 0)
	return
}

func (d *Device) configRw(offset, wv, nBytes uint, isWrite bool) (rv uint, err error) {
	var f *os.File
	f, err = d.SysfsOpenFile("config", os.O_RDWR)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err = f.Seek(int64(offset), os.SEEK_SET); err != nil {
		return
	}
	var b [4]byte
	if isWrite {
		for i := range b {
			b[i] = byte((wv >> uint(8*i)) & 0xff)
		}
		_, err = f.Write(b[:nBytes])
		rv = wv
	} else {
		_, err = f.Read(b[:nBytes])
		if err == nil {
			for i := range b {
				rv |= uint(b[i]) << (8 * uint(i))
			}
		}
	}
	return
}

func (d *Device) ReadConfigUint32(offset uint) (v uint32, err error) {
	var i uint
	i, err = d.configRw(offset, 0, 4, false)
	v = uint32(i)
	return
}
func (d *Device) WriteConfigUint32(offset uint, value uint32) (err error) {
	_, err = d.configRw(offset, uint(value), 4, true)
	return
}
func (d *Device) ReadConfigUint16(offset uint) (v uint16, err error) {
	var i uint
	i, err = d.configRw(offset, 0, 2, false)
	v = uint16(i)
	return
}
func (d *Device) WriteConfigUint16(offset uint, value uint16) (err error) {
	_, err = d.configRw(offset, uint(value), 2, true)
	return
}

func (d *Device) MapResource(r *Resource) (err error) {
	var f *os.File
	f, err = d.SysfsOpenFile("resource%d", os.O_RDWR, r.Index)
	if err != nil {
		return
	}
	defer f.Close()
	r.Mem, err = syscall.Mmap(int(f.Fd()), 0, int(r.Size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap resource%d: %s", r.Index, err)
	}
	return
}

func (d *Device) UnmapResource(r *Resource) (err error) {
	err = syscall.Munmap(r.Mem)
	if err != nil {
		return fmt.Errorf("munmap resource%d: %s", r.Index, err)
	}
	return
}

func DiscoverDevices() (err error) {
	fis, err := ioutil.ReadDir(sysBusPciPath)
	if perr, ok := err.(*os.PathError); ok && perr.Err == syscall.ENOENT {
		return
	}
	if err != nil {
		return
	}
	for _, fi := range fis {
		de := NewDevice()
		d := de.GetDevice()
		n := fi.Name()
		if _, err = fmt.Sscanf(n, "%x:%x:%x.%x", &d.Addr.Domain, &d.Addr.Bus, &d.Addr.Slot, &d.Addr.Fn); err != nil {
			return
		}

		var f *os.File
		f, err = d.SysfsOpenFile("config", os.O_RDONLY)
		if err != nil {
			return
		}
		defer f.Close()
		d.configBytes, err = ioutil.ReadAll(f)

		r := bytes.NewReader(d.configBytes)
		binary.Read(r, binary.LittleEndian, &d.Config)
		if d.Config.Hdr.Type() != Normal {
			continue
		}

		driver := GetDriver(d.Config.Hdr.DeviceID)
		if driver == nil {
			continue
		}

		// Loop through BARs to find resources.
		{
			i := 0
			for i < len(d.Config.BaseAddressRegs) {
				bar := d.Config.BaseAddressRegs[i]
				if !bar.Valid() {
					i++
					continue
				}
				var rfi os.FileInfo
				rfi, err = os.Stat(d.SysfsPath("resource%d", i))
				if err != nil {
					return
				}
				r := Resource{
					Index: uint32(i),
					Base:  uint64(bar.Addr()),
					Size:  uint64(rfi.Size()),
				}
				r.BAR[0] = bar

				i++
				is64bit := (bar>>1)&3 == 2
				if is64bit {
					r.BAR[1] = d.Config.BaseAddressRegs[i+1]
					r.Base |= uint64(r.BAR[1]) << 32
					i++
				}

				d.Resources = append(d.Resources, r)
			}
		}

		d.Driver = driver
		d.DriverDevice, err = driver.DeviceMatch(d)
		if err != nil {
			return
		}

		if d.DriverDevice == nil {
			continue
		}

		// Open and initialize matched device.
		err = d.Devicer.Open()
		if err != nil {
			return
		}
		d.DriverDevice.Init()
	}
	return
}
