package autofix

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"time"
)

func ElseIf(ctx context.Context, id int, first bool, query func(context.Context, int) error) error {
	if first {
		return nil
	} else if err := query(ctx, id); err != nil {
		return err
	}
	return nil
}

func OuterError(call func() error) error {
	err := call()
	if err != nil {
		return err
	}
	if err := call(); err != nil {
		return err
	}
	return err
}

func Timeout(ctx context.Context, call func(context.Context) error) error {
	x, cancel := context.WithCancel(ctx)
	defer cancel()
	x, cancel = context.WithTimeout(x, time.Second)
	defer cancel()
	if err := call(x); err != nil {
		return err
	}
	return x.Err()
}

func Imports(input []byte, call func(string) error) error {
	buffer := bytes.NewBuffer(input)
	if err := call(hex.EncodeToString(buffer.Bytes())); err != nil {
		return errors.Join(err, errors.New("encode"))
	}
	return nil
}
