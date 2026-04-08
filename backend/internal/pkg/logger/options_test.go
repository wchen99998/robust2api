package logger

import "testing"

func TestNormalizedOptions_StdoutOnly(t *testing.T) {
	opts := InitOptions{
		Level:           "TRACE",
		Format:          "TEXT",
		ServiceName:     "",
		Environment:     "",
		StacktraceLevel: "panic",
		Output: OutputOptions{
			ToStdout: false,
		},
		Sampling: SamplingOptions{
			Enabled:    true,
			Initial:    0,
			Thereafter: 0,
		},
	}

	out := opts.normalized()
	if out.Level != "trace" {
		t.Fatalf("normalized level should preserve value for upstream validation, got %q", out.Level)
	}
	if !out.Output.ToStdout {
		t.Fatalf("normalized output should always use stdout")
	}
	if out.Sampling.Initial != 100 || out.Sampling.Thereafter != 100 {
		t.Fatalf("normalized sampling defaults invalid: %+v", out.Sampling)
	}
}

func TestBootstrapOptions_StdoutOnly(t *testing.T) {
	opts := bootstrapOptions()
	if !opts.Output.ToStdout {
		t.Fatalf("bootstrap logger must write to stdout")
	}
}
