package streamdeck

import (
	"errors"
	"fmt"
	"image"
	"image/color"

	"github.com/karalabe/hid"
)

const vendorID = 0x0fd9

// deviceType represents one of the various types of StreamDeck (mini/orig/orig2/xl)
type deviceType struct {
	name                string
	imageSize           image.Point
	usbProductID        uint16
	resetPacket         []byte
	numberOfButtons     uint
	buttonRows          uint
	buttonCols          uint
	brightnessPacket    []byte
	buttonReadOffset    uint
	imageFormat         string
	imagePayloadPerPage uint
	imageHeaderFunc     func(bytesRemaining uint, btnIndex uint, pageNumber uint) []byte
}

var deviceTypes []deviceType

// RegisterDevicetype allows the declaration of a new type of device, intended for use by subpackage "devices"
func RegisterDevicetype(
	name string,
	imageSize image.Point,
	usbProductID uint16,
	resetPacket []byte,
	numberOfButtons uint,
	buttonRows uint,
	buttonCols uint,
	brightnessPacket []byte,
	buttonReadOffset uint,
	imageFormat string,
	imagePayloadPerPage uint,
	imageHeaderFunc func(bytesRemaining uint, btnIndex uint, pageNumber uint) []byte,
) {
	d := deviceType{
		name:                name,
		imageSize:           imageSize,
		usbProductID:        usbProductID,
		resetPacket:         resetPacket,
		numberOfButtons:     numberOfButtons,
		buttonRows:          buttonRows,
		buttonCols:          buttonCols,
		brightnessPacket:    brightnessPacket,
		buttonReadOffset:    buttonReadOffset,
		imageFormat:         imageFormat,
		imagePayloadPerPage: imagePayloadPerPage,
		imageHeaderFunc:     imageHeaderFunc,
	}
	deviceTypes = append(deviceTypes, d)
}

// Device is a struct which represents an actual Streamdeck device, and holds its reference to the USB HID device
type Device struct {
	fd                     *hid.Device
	deviceType             deviceType
	buttonPressListeners   []func(int, *Device, error)
	buttonReleaseListeners []func(int, *Device, error)
}

// Open a Streamdeck device, the most common entry point
func Open() (*Device, error) {
	return rawOpen(true)
}

// OpenWithoutReset will open a Streamdeck device, without resetting it
func OpenWithoutReset() (*Device, error) {
	return rawOpen(false)
}

// Open a Streamdeck device with a specific ProductID
func OpenWithID(productID uint16) (*Device, error) {
	return rawOpenWithID(true, productID)
}

// Opens a new Streamdeck device, and returns a handle
func rawOpenWithID(reset bool, productID uint16) (*Device, error) {
	devices := hid.Enumerate(vendorID, 0)
	if len(devices) == 0 {
		return nil, errors.New("No elgato devices found")
	}

	retval := &Device{}
	deviceConnected := false
	for _, device := range devices {
		if device.ProductID != productID {
			continue
		}

		for _, devType := range deviceTypes {
			if device.ProductID == devType.usbProductID {
				retval.deviceType = devType
				dev, err := device.Open()
				if err != nil {
					return nil, err
				}
				retval.fd = dev
				if reset {
					retval.ResetComms()
				}
				go retval.buttonPressListener()
				return retval, nil
			}
		}

		deviceConnected = true
		break
	}
	if !deviceConnected {
		return nil, errors.New("No device connected with given product ID.")
	}

	return nil, errors.New("No device registered for requested product ID.")
}

// Opens a new StreamdeckXL device, and returns a handle
func rawOpen(reset bool) (*Device, error) {
	devices := hid.Enumerate(vendorID, 0)
	if len(devices) == 0 {
		return nil, errors.New("No elgato devices found")
	}

	retval := &Device{}
	for _, device := range devices {
		// Iterate over the known device types, matching to product ID
		for _, devType := range deviceTypes {
			if device.ProductID == devType.usbProductID {
				retval.deviceType = devType
				dev, err := device.Open()
				if err != nil {
					return nil, err
				}
				retval.fd = dev
				if reset {
					retval.ResetComms()
				}
				go retval.buttonPressListener()
				return retval, nil
			}
		}
	}
	return nil, errors.New("Found an Elgato device, but not one for which there is a definition; have you imported the devices package?")
}

// GetName returns the name of the type of Streamdeck
func (d *Device) GetName() string {
	return d.deviceType.name
}

// GetProductID returns the product ID of the type of Streamdeck
func (d *Device) GetProductID() uint16 {
	return d.deviceType.usbProductID
}

// Close the device
func (d *Device) Close() {
	d.fd.Close()
}

// SetBrightness sets the button brightness
// pct is an integer between 0-100
func (d *Device) SetBrightness(pct int) error {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	preamble := d.deviceType.brightnessPacket
	payload := append(preamble, byte(pct))
	_, err := d.fd.SendFeatureReport(payload)
	if err != nil {
		return err
	}
	return nil
}

// GetButtonImageSize returns the size of the images to uploaded to the buttons
func (d *Device) GetButtonImageSize() image.Point {
	return d.deviceType.imageSize
}

// GetNumButtonsOnDevice returns the number of button this device has
func (d *Device) GetNumButtonsOnDevice() uint {
	return d.deviceType.numberOfButtons
}

// ClearButtons writes a black square to all buttons
func (d *Device) ClearButtons() error {
	numButtons := int(d.deviceType.numberOfButtons)
	for i := 0; i < numButtons; i++ {
		err := d.WriteColorToButton(i, color.Black)
		if err != nil {
			return err
		}
	}
	return nil
}

// WriteColorToButton writes a specified color to the given button
func (d *Device) WriteColorToButton(btnIndex int, colour color.Color) error {
	img := getSolidColourImage(colour, d.deviceType.imageSize.X)
	imgForButton, err := getImageForButton(img, d.deviceType.imageFormat)
	if err != nil {
		return err
	}
	return d.rawWriteToButton(btnIndex, imgForButton)
}

// WriteImageToButton writes a specified image file to the given button
func (d *Device) WriteImageToButton(btnIndex int, filename string) error {
	img, err := getImageFile(filename)
	if err != nil {
		return err
	}
	err = d.WriteRawImageToButton(btnIndex, img)
	if err != nil {
		return err
	}
	return nil
}

func (d *Device) buttonPressListener() {
	var buttonMask []bool
	buttonMask = make([]bool, d.deviceType.numberOfButtons)
	for {
		data := make([]byte, d.deviceType.numberOfButtons+d.deviceType.buttonReadOffset)
		_, err := d.fd.Read(data)
		if err != nil {
			d.sendButtonPressEvent(-1, err)
			break
		}
		for i := uint(0); i < d.deviceType.numberOfButtons; i++ {
			if data[d.deviceType.buttonReadOffset+i] == 1 {
				if !buttonMask[i] {
					d.sendButtonPressEvent(int(i), nil)
				}
				buttonMask[i] = true
			} else {
				if buttonMask[i] {
					d.sendButtonReleaseEvent(int(i), nil)
				}
				buttonMask[i] = false
			}
		}
	}
}

func (d *Device) sendButtonPressEvent(btnIndex int, err error) {
	for _, f := range d.buttonPressListeners {
		f(btnIndex, d, err)
	}
}

func (d *Device) sendButtonReleaseEvent(btnIndex int, err error) {
	for _, f := range d.buttonReleaseListeners {
		f(btnIndex, d, err)
	}
}

// ButtonPress registers a callback to be called whenever a button is pressed
func (d *Device) ButtonPress(f func(int, *Device, error)) {
	d.buttonPressListeners = append(d.buttonPressListeners, f)
}

// ButtonRelease registers a callback to be called whenever a button is released
func (d *Device) ButtonRelease(f func(int, *Device, error)) {
	d.buttonReleaseListeners = append(d.buttonReleaseListeners, f)
}

// ResetComms will reset the comms protocol to the StreamDeck; useful if things have gotten de-synced, but it will also reboot the StreamDeck
func (d *Device) ResetComms() error {
	payload := d.deviceType.resetPacket
	_, err := d.fd.SendFeatureReport(payload)
	return err
}

// WriteRawImageToButton takes an `image.Image` and writes it to the given button, after resizing and rotating the image to fit the button (for some reason the StreamDeck screens are all upside down)
func (d *Device) WriteRawImageToButton(btnIndex int, rawImg image.Image) error {
	img := resizeAndRotate(rawImg, d.deviceType.imageSize.X, d.deviceType.imageSize.Y, d.deviceType.name)
	imgForButton, err := getImageForButton(img, d.deviceType.imageFormat)
	if err != nil {
		return err
	}
	return d.rawWriteToButton(btnIndex, imgForButton)
}

func (d *Device) rawWriteToButton(btnIndex int, rawImage []byte) error {
	// Based on set_key_image from https://github.com/abcminiuser/python-elgato-streamdeck/blob/master/src/StreamDeck/Devices/StreamDeckXL.py#L151

	if Min(Max(btnIndex, 0), int(d.deviceType.numberOfButtons)) != btnIndex {
		return errors.New(fmt.Sprintf("Invalid key index: %d", btnIndex))
	}

	pageNumber := 0
	bytesRemaining := len(rawImage)
	halfImage := len(rawImage) / 2
	bytesSent := 0

	for bytesRemaining > 0 {

		header := d.deviceType.imageHeaderFunc(uint(bytesRemaining), uint(btnIndex), uint(pageNumber))
		imageReportLength := int(d.deviceType.imagePayloadPerPage)
		imageReportHeaderLength := len(header)
		imageReportPayloadLength := imageReportLength - imageReportHeaderLength

		/*
			if halfImage > imageReportPayloadLength {
				log.Fatalf("image too large: %d", halfImage*2)
			}
		*/

		thisLength := 0
		if imageReportPayloadLength < bytesRemaining {
			if d.deviceType.name == "Stream Deck Original" {
				thisLength = halfImage
			} else {
				thisLength = imageReportPayloadLength
			}
		} else {
			thisLength = bytesRemaining
		}

		payload := append(header, rawImage[bytesSent:(bytesSent+thisLength)]...)
		padding := make([]byte, imageReportLength-len(payload))

		thingToSend := append(payload, padding...)
		_, err := d.fd.Write(thingToSend)
		if err != nil {
			return err
		}

		bytesRemaining = bytesRemaining - thisLength
		pageNumber = pageNumber + 1
		bytesSent = bytesSent + thisLength
	}
	return nil
}

// Golang Min/Max
func Min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func Max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
