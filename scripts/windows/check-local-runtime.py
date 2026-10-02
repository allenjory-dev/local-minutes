"""Read-only checks using the provisioned Local Minutes interpreter, never pip/uv."""
import json
from importlib.metadata import version

import torch
import whisperx  # noqa: F401

if not torch.cuda.is_available():
    raise RuntimeError("CUDA is unavailable; no installation or CPU fallback was attempted")
if torch.ones(2, device="cuda").sum().item() != 2:
    raise RuntimeError("CUDA computation failed")
print(json.dumps({
    "local_cuda_preflight": "PASS",
    "gpu": torch.cuda.get_device_name(0),
    "packages": {name: version(name) for name in
                 ("torch", "whisperx", "faster-whisper", "ctranslate2")},
}))
