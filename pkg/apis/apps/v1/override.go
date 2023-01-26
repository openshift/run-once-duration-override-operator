package v1

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (in *RunOnceDurationOverride) IsTimeToRotateCert() bool {
	if in.Status.CertsRotateAt.IsZero() {
		return true
	}

	now := metav1.Now()
	if in.Status.CertsRotateAt.Before(&now) {
		return true
	}

	return false
}

func (in *PodResourceOverrideSpec) String() string {
	return fmt.Sprintf("ActiveDeadlineSecondsOverride=%d", in.ActiveDeadlineSecondsOverride)
}

func (in *PodResourceOverrideSpec) Validate() error {
	if in.ActiveDeadlineSecondsOverride < 0 {
		return errors.New("invalid value for ActiveDeadlineSecondsOverride, must be a positive value")
	}

	return nil
}

func (in *PodResourceOverrideSpec) Hash() string {
	value := fmt.Sprintf("%s", in)

	writer := sha256.New()
	writer.Write([]byte(value))
	return hex.EncodeToString(writer.Sum(nil))
}
