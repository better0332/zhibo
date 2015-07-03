package flv

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

const (
	HEADER_LENGTH     = 9
	TAG_HEADER_LENGTH = 11
)

type Flv struct {
	r            io.Reader
	restHeadTag  []byte
	restKeyFrame []byte
}

func NewFlv(reader io.Reader) *Flv {
	return &Flv{r: reader}
}

func (flv *Flv) BasicFlvBlock() ([]byte, error) {
	var err error
	buf := make([]byte, HEADER_LENGTH+4)
	if _, err = io.ReadFull(flv.r, buf); err != nil {
		return nil, err
	}
	if string(buf[:3]) != "FLV" {
		return nil, errors.New("bad flv file format")
	}

	data := make([]byte, 0, 1024) // default 1k
	data = append(data, buf...)

	var v int32

	buf = make([]byte, TAG_HEADER_LENGTH)
	if _, err = io.ReadFull(flv.r, buf); err != nil {
		return nil, err
	}
	if buf[0] != '\x12' {
		return nil, errors.New("not found Metadata Tag")
	}
	data = append(data, buf...)
	binary.Read(bytes.NewReader(buf[:4]), binary.BigEndian, &v)
	metaSize := v & 0x00FFFFFF
	buf = make([]byte, metaSize+4)
	if _, err = io.ReadFull(flv.r, buf); err != nil {
		return nil, err
	}
	data = append(data, buf...)

	// AVCDecoderConfigurationRecord
	buf = make([]byte, TAG_HEADER_LENGTH)
	if _, err = io.ReadFull(flv.r, buf); err != nil {
		return nil, err
	}
	if buf[0] != '\x09' {
		return nil, errors.New("not found Video Tag")
	}
	data = append(data, buf...)
	binary.Read(bytes.NewReader(buf[:4]), binary.BigEndian, &v)
	videoSize := v & 0x00FFFFFF
	buf = make([]byte, videoSize+4)
	if _, err = io.ReadFull(flv.r, buf); err != nil {
		return nil, err
	}
	if buf[0] != '\x17' {
		return nil, errors.New("not keyframe or AVC")
	}
	data = append(data, buf...)

	buf = make([]byte, TAG_HEADER_LENGTH)
	if _, err = io.ReadFull(flv.r, buf); err != nil {
		return nil, err
	}
	if buf[0] == '\x08' {
		// AudioSpecificConfig
		data = append(data, buf...)
		binary.Read(bytes.NewReader(buf[:4]), binary.BigEndian, &v)
		audioSize := v & 0x00FFFFFF
		buf = make([]byte, audioSize+4)
		if _, err = io.ReadFull(flv.r, buf); err != nil {
			return nil, err
		}
		data = append(data, buf...)
		return data, nil
	} else if buf[0] == '\x09' {
		// no AudioSpecificConfig
		flv.restHeadTag = buf
		return data, nil
	} else {
		return nil, errors.New("unknow Tag")
	}
}

func (flv *Flv) Read() ([]byte, error) {
	var err error
	if flv.restHeadTag == nil {
		flv.restHeadTag = make([]byte, TAG_HEADER_LENGTH)
		if _, err = io.ReadFull(flv.r, flv.restHeadTag); err != nil {
			return nil, err
		}
	}
	if flv.restHeadTag[0] != '\x09' {
		return nil, errors.New("not found Video Tag")
	}

	data := make([]byte, 0, 200<<10) //default 200k
	data = append(data, flv.restHeadTag...)

	var tagSize int32

	if flv.restKeyFrame == nil {
		binary.Read(bytes.NewReader(flv.restHeadTag[:4]), binary.BigEndian, &tagSize)
		tagSize = tagSize & 0x00FFFFFF
		flv.restKeyFrame = make([]byte, tagSize+4)
		if _, err = io.ReadFull(flv.r, flv.restKeyFrame); err != nil {
			return nil, err
		}
	}
	if flv.restKeyFrame[0] != '\x17' {
		return nil, errors.New("not keyframe or AVC")
	}
	data = append(data, flv.restKeyFrame...)

	flv.restHeadTag = make([]byte, TAG_HEADER_LENGTH)
	for {
		if _, err = io.ReadFull(flv.r, flv.restHeadTag); err != nil {
			return nil, err
		}
		binary.Read(bytes.NewReader(flv.restHeadTag[:4]), binary.BigEndian, &tagSize)
		tagSize = tagSize & 0x00FFFFFF
		flv.restKeyFrame = make([]byte, tagSize+4)
		if _, err = io.ReadFull(flv.r, flv.restKeyFrame); err != nil {
			return nil, err
		}
		if flv.restHeadTag[0] == '\x09' && flv.restKeyFrame[0] == '\x17' {
			// Keyframe Video Tag
			return data, nil
		}
		data = append(data, flv.restHeadTag...)
		data = append(data, flv.restKeyFrame...)
	}
}
