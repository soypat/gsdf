package gleval

import "errors"

type BatcherConfig struct {
	GPUCompute ComputeConfig
}

func (b *Batcher) ExecuteRawBinaryOperation(opSource []byte, dst, A, B []float32) error {
	return b.runBinop(string(opSource), b.cfg, dst, A, B)
}

func (b *Batcher) Configure(cfg BatcherConfig) error {
	if cfg.GPUCompute.InvocX <= 0 {
		return errors.New("invalid compute InvocX")
	} else if len(cfg.GPUCompute.ShaderObjects) > 0 {
		return errors.New("shader objects unsupported as of yet for Batcher")
	}
	b.cfg = cfg.GPUCompute
	return nil
}

type Batcher struct {
	cfg         ComputeConfig
	shaderStore []byte
}

const baseBinOpShader = `#version 430
layout(local_size_x = %d, local_size_y = 1, local_size_z = 1) in;

// Input1 of binary operation.
layout(std430, binding = 0) buffer ABuffer {
	float vbo_a[];
};

// Input2 of binary operation.
layout(std430, binding = 1) buffer BBuffer {
	float vbo_b[];
};

// Output buffer result of binary operation.
layout(std430, binding = 2) buffer OutBuffer {
	float vbo_out[];
};

float op(float a, float b) {
%s
}

void main() {
	int idx = int( gl_GlobalInvocationID.x );
	vbo_out[idx] = op(vbo_a[idx], vbo_b[idx]);
}`
