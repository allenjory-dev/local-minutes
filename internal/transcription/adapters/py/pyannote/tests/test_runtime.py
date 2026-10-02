"""Run with Python 3.11+ in the isolated pyannote environment; FFmpeg required."""
import hashlib
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import tomllib
import unittest

from packaging.markers import Marker, default_environment
import torch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from pyannote_diarize import load_audio_for_diarization


class RuntimeTests(unittest.TestCase):
    def test_platform_wheel_selection(self):
        config = tomllib.loads((ROOT / "pyproject.toml").read_text())
        for system, machine, expected in [
            ("win32", "AMD64", "pytorch"),
            ("win32", "x86_64", "pytorch"),
            ("linux", "x86_64", "pytorch"),
            ("linux", "aarch64", "pytorch-cpu"),
            ("darwin", "arm64", "pytorch-cpu"),
            ("darwin", "x86_64", "pytorch-cpu"),
        ]:
            env = default_environment()
            env.update(sys_platform=system, platform_machine=machine)
            for package in ("torch", "torchaudio"):
                matches = [entry["index"] for entry in config["tool"]["uv"]["sources"][package]
                           if Marker(entry["marker"]).evaluate(env)]
                self.assertEqual(matches, [expected], (system, machine, package))

    def test_decodes_stereo_resampled_audio_without_changing_source(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = Path(tmp) / "stereo source.wav"
            subprocess.run(["ffmpeg", "-nostdin", "-v", "error", "-n", "-f", "lavfi", "-i",
                            "sine=frequency=440:duration=0.1", "-ar", "48000", "-ac", "2", str(source)], check=True)
            before = hashlib.sha256(source.read_bytes()).digest()
            audio = load_audio_for_diarization(str(source))
            self.assertEqual(audio["sample_rate"], 16000)
            self.assertEqual(tuple(audio["waveform"].shape), (1, 1600))
            self.assertEqual(audio["waveform"].dtype, torch.float32)
            self.assertTrue(torch.isfinite(audio["waveform"]).all())
            self.assertGreater(audio["waveform"].abs().max().item(), 0)
            self.assertEqual(hashlib.sha256(source.read_bytes()).digest(), before)

    def test_invalid_audio_fails_without_changing_source(self):
        with tempfile.TemporaryDirectory() as tmp:
            source = Path(tmp) / "broken.wav"
            source.write_bytes(b"not an audio file")
            with self.assertRaises(RuntimeError):
                load_audio_for_diarization(str(source))
            self.assertEqual(source.read_bytes(), b"not an audio file")


if __name__ == "__main__":
    unittest.main()
