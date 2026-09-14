package chattool_test

import "fmt"

type statusError struct {
	statusCode int
	message    string
}

func (e statusError) Error() string {
	if e.message != "" {
		return e.message
	}
	return fmt.Sprintf("status %d", e.statusCode)
}

func (e statusError) StatusCode() int {
	return e.statusCode
}
