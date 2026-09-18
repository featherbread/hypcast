package atsc

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	validChannelsConf = `KCTS-HD:189000000:8VSB:49:52:3
KIDS:189000000:8VSB:65:68:4
CREATE:189000000:8VSB:81:84:5
WORLD:189000000:8VSB:97:100:6`

	validChannelsConfNonstandard8VSB = "KCTS-HD:189000000:VSB_8:49:52:3"
	validChannelsConfQAM64           = "Test QAM 64:255000000:QAM_64:42:43:5"
	validChannelsConfQAM256          = "WLFI:255000000:QAM_256:66:68:4"
)

func TestParseChannelsConf(t *testing.T) {
	testCases := []struct {
		Description string
		Input       string
		Want        []Channel
		WantErr     bool
	}{
		{
			Description: "valid channels.conf",
			Input:       validChannelsConf,
			Want: []Channel{
				{"KCTS-HD", 189_000_000, Modulation8VSB, 49, 52, 3},
				{"KIDS", 189_000_000, Modulation8VSB, 65, 68, 4},
				{"CREATE", 189_000_000, Modulation8VSB, 81, 84, 5},
				{"WORLD", 189_000_000, Modulation8VSB, 97, 100, 6},
			},
		},

		{
			Description: "w_scan2 nonstandard 8VSB output",
			Input:       validChannelsConfNonstandard8VSB,
			Want: []Channel{
				{"KCTS-HD", 189_000_000, Modulation8VSB, 49, 52, 3},
			},
		},

		{
			Description: "QAM64 modulation",
			Input:       validChannelsConfQAM64,
			Want: []Channel{
				{"Test QAM 64", 255_000_000, ModulationQAM64, 42, 43, 5},
			},
		},

		{
			Description: "QAM256 modulation",
			Input:       validChannelsConfQAM256,
			Want: []Channel{
				{"WLFI", 255_000_000, ModulationQAM256, 66, 68, 4},
			},
		},

		{
			Description: "wrong number of fields",
			Input:       "KCTS-HD:189000000:8VSB:3",
			WantErr:     true,
		},

		{
			Description: "invalid frequency",
			Input:       "KCTS-HD:189.0123456:8VSB:49:52:3",
			WantErr:     true,
		},

		{
			Description: "invalid modulation",
			Input:       "KCTS-HD:189000000:42VSB:49:52:3",
			WantErr:     true,
		},

		{
			Description: "invalid PID",
			Input:       "KCTS-HD:189000000:8VSB:49:52:?",
			WantErr:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			got, err := ParseChannelsConf(strings.NewReader(tc.Input))
			if tc.WantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if err == nil {
				assert.Equal(t, tc.Want, got)
			}
		})
	}
}

func FuzzParseChannelsConf(f *testing.F) {
	f.Add(validChannelsConf)
	f.Add(validChannelsConfNonstandard8VSB)
	f.Add(validChannelsConfQAM64)
	f.Add(validChannelsConfQAM256)

	f.Fuzz(func(t *testing.T, inputStringConf string) {
		parsed, err := ParseChannelsConf(strings.NewReader(inputStringConf))
		if err != nil {
			t.SkipNow()
		}
		reencoded := formatChannelsConf(t, parsed)
		reparsed, err := ParseChannelsConf(strings.NewReader(reencoded))
		require.NoError(t, err, "Encoded channel list should re-parse")
		assert.Equal(t, parsed, reparsed, "channels.conf should round-trip without changes")
	})
}

func formatChannelsConf(t *testing.T, channels []Channel) string {
	t.Helper()
	var buf strings.Builder
	for _, ch := range channels {
		buf.WriteString(ch.String())
		buf.WriteRune('\n')
	}
	return buf.String()
}
