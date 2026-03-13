package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMaskSensitiveContentMasksCommonPersonalData(t *testing.T) {
	input := "이메일 jane.doe@example.com, 휴대폰 010-1234-5678, 주민번호 900101-2345678, 카드 4111 1111 1111 1111"

	masked := maskSensitiveContent(input)

	require.NotContains(t, masked, "jane.doe@example.com")
	require.Contains(t, masked, "ja******@example.com")
	require.Contains(t, masked, "010-****-5678")
	require.Contains(t, masked, "******-*******")
	require.Contains(t, masked, "**** **** **** 1111")
}

func TestMaskSensitiveContentMasksResidentIDWithoutHyphen(t *testing.T) {
	input := "주민번호 9001011234567 와 외국인번호 0101015234567"

	masked := maskSensitiveContent(input)

	require.NotContains(t, masked, "9001011234567")
	require.NotContains(t, masked, "0101015234567")
	require.Contains(t, masked, "*************")
}
