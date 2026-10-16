package openrouter

import (
	"context"
	"errors"
)

type faultError struct {
	class string
}

var (
	errCanceled        error = faultError{class: FaultCanceled}
	errCredit          error = faultError{class: FaultCredit}
	errInvalidInput    error = faultError{class: FaultInvalidInput}
	errInvalidOutput   error = faultError{class: FaultInvalidOutput}
	errOverload        error = faultError{class: FaultOverload}
	errRemote          error = faultError{class: FaultRemote}
	errTemporary       error = faultError{class: FaultTemporary}
	errTemporaryCredit error = faultError{class: FaultTemporaryCredit}
	errTimeout         error = faultError{class: FaultTimeout}
	errTransport       error = faultError{class: FaultTransport}
)

func (e faultError) Error() string {
	return e.class
}

func (e faultError) FaultClass() string {
	return e.class
}

func newFault(class string) error {
	switch class {
	case FaultCanceled:
		return errCanceled
	case FaultCredit:
		return errCredit
	case FaultInvalidInput:
		return errInvalidInput
	case FaultInvalidOutput:
		return errInvalidOutput
	case FaultOverload:
		return errOverload
	case FaultTemporary:
		return errTemporary
	case FaultTemporaryCredit:
		return errTemporaryCredit
	case FaultTimeout:
		return errTimeout
	case FaultTransport:
		return errTransport
	default:
		return errRemote
	}
}

func faultClass(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FaultTimeout
	}
	if errors.Is(err, context.Canceled) {
		return FaultCanceled
	}
	var classified interface{ FaultClass() string }
	if errors.As(err, &classified) {
		return classified.FaultClass()
	}
	return FaultRemote
}

func requestFault(ctx context.Context, err error) error {
	if contextErr := contextFault(ctx.Err()); contextErr != nil {
		return contextErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return newFault(FaultTimeout)
	}
	if errors.Is(err, context.Canceled) {
		return newFault(FaultCanceled)
	}
	return newFault(FaultTransport)
}

func contextFault(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return newFault(FaultTimeout)
	case errors.Is(err, context.Canceled):
		return newFault(FaultCanceled)
	default:
		return nil
	}
}

func SafeFaultClass(err error) string {
	return faultClass(err)
}
