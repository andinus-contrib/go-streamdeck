package streamdeck

import "image"

// ButtonDisplay is the interface to satisfy for displaying on a button
type ButtonDisplay interface {
	GetImageForButton(int) image.Image
	GetButtonIndex() int
	SetButtonIndex(int)
	RegisterUpdateHandler(func(Button))
	Pressed()
}

// ButtonActionHandler is the interface to satisfy for handling a button being pressed, generally via an `actionhandler`
type ButtonActionHandler interface {
	Pressed(Button)
}

// Button is the interface to satisfy for being a button; currently this is a direct proxy for the `ButtonDisplay` interface as there isn't a requirement to handle being pressed
type Button interface {
	ButtonDisplay
}

// ButtonDecorator represents a way to modify the button image, for example to add a highlight or an "on/off" hint
type ButtonDecorator interface {
	Apply(image.Image, int) image.Image
}

// StreamDeck is the main struct to represent a StreamDeck device, and internally contains the reference to a `Device`
type StreamDeck struct {
	dev        *Device
	buttons    map[int]Button
	decorators map[int]ButtonDecorator
}

// New will return a new instance of a `StreamDeck`, and is the main entry point for the higher-level interface.  It will return an error if there is no StreamDeck plugged in.
func New() (*StreamDeck, error) {
	sd := &StreamDeck{}
	d, err := Open()
	if err != nil {
		return nil, err
	}
	sd.dev = d
	sd.buttons = make(map[int]Button)
	sd.decorators = make(map[int]ButtonDecorator)
	sd.dev.ButtonPress(sd.pressHandler)
	return sd, nil
}

// NewWithID will return a new instance of a `StreamDeck`.  It will return an error if there is no StreamDeck plugged in with the given ID.
func NewWithID(productID uint16) (*StreamDeck, error) {
	sd := &StreamDeck{}
	d, err := OpenWithID(productID)
	if err != nil {
		return nil, err
	}
	sd.dev = d
	sd.buttons = make(map[int]Button)
	sd.decorators = make(map[int]ButtonDecorator)
	sd.dev.ButtonPress(sd.pressHandler)
	return sd, nil
}

// GetName returns the name of the type of Streamdeck
func (sd *StreamDeck) GetName() string {
	return sd.dev.deviceType.name
}

// AddButton adds a `Button` object to the StreamDeck at the specified index
func (sd *StreamDeck) AddButton(btnIndex int, b Button) {
	b.RegisterUpdateHandler(sd.ButtonUpdateHandler)
	b.SetButtonIndex(btnIndex)
	sd.buttons[btnIndex] = b
	sd.updateButton(b)
}

// SetDecorator imposes a ButtonDecorator onto a given button
func (sd *StreamDeck) SetDecorator(btnIndex int, d ButtonDecorator) {
	sd.decorators[btnIndex] = d
	// If there's a button there, update it
	btn, ok := sd.buttons[btnIndex]
	if ok {
		sd.updateButton(btn)
	}
}

// UnsetDecorator removes a ButtonDecorator from a given button
func (sd *StreamDeck) UnsetDecorator(btnIndex int) {
	delete(sd.decorators, btnIndex)
	// If there's a button there, update it
	btn, ok := sd.buttons[btnIndex]
	if ok {
		sd.updateButton(btn)
	}
}

// ButtonUpdateHandler allows a user of this library to signal when something external has changed, such that this button should be update
func (sd *StreamDeck) ButtonUpdateHandler(b Button) {
	sd.buttons[b.GetButtonIndex()] = b
	sd.updateButton(b)
}

// GetButtonByIndex returns a button for the given index
func (sd *StreamDeck) GetButtonIndex(btnIndex int) Button {
	b, ok := sd.buttons[btnIndex]
	if !ok {
		return nil
	}
	return b
}

func (sd *StreamDeck) pressHandler(btnIndex int, d *Device, err error) {
	if err != nil {
		panic(err)
	}
	b := sd.buttons[btnIndex]
	if b != nil {
		sd.buttons[btnIndex].Pressed()
	}
}

func (sd *StreamDeck) updateButton(b Button) error {
	img := b.GetImageForButton(sd.dev.deviceType.imageSize.X)
	decorator, ok := sd.decorators[b.GetButtonIndex()]
	if ok {
		img = decorator.Apply(img, sd.dev.deviceType.imageSize.X)
	}
	e := sd.dev.WriteRawImageToButton(b.GetButtonIndex(), img)
	return e
}

func (sd *StreamDeck) SetBrightness(brightness int) {
	sd.dev.SetBrightness(brightness)
}
