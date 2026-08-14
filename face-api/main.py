"""
Mini-DSS foto validatsiya servisi.

Maqsad: rasmni terminalga yuborishdan OLDIN tekshirish. 9801 ta rasmni
tekshirmasdan yuborsak, sync paytida ko'p xatolik chiqadi va qaysi biri nega
rad etilganini bilib bo'lmaydi.

Chegaralar terminalning o'z config'idan olingan —
device-probe/DEVICE-REPORT.md 8.3-band (VideoAnalyseRule[0][0]):
    EyesDistThreshold = 60
    MinQuality       = 50
    SizeFilter.MinSize = 700x700 (8192 normalizatsiyada)
"""
from __future__ import annotations

import io
import os
from dataclasses import dataclass, field

import numpy as np
from fastapi import FastAPI, File, UploadFile
from PIL import Image, ImageOps

MIN_EYE_DISTANCE = int(os.getenv("MIN_EYE_DISTANCE", "60"))
MIN_QUALITY = int(os.getenv("MIN_QUALITY", "50"))
MAX_YAW = float(os.getenv("MAX_YAW", "25"))
MAX_PITCH = float(os.getenv("MAX_PITCH", "25"))
MAX_ROLL = float(os.getenv("MAX_ROLL", "25"))
MAX_UPLOAD_BYTES = int(os.getenv("MAX_UPLOAD_BYTES", str(10 * 1024 * 1024)))

app = FastAPI(title="Mini-DSS face validation", version="1.0")

_analyzer = None


def get_analyzer():
    """InsightFace modelini birinchi so'rovda yuklaymiz (~300 MB)."""
    global _analyzer
    if _analyzer is None:
        from insightface.app import FaceAnalysis

        _analyzer = FaceAnalysis(name="buffalo_l", providers=["CPUExecutionProvider"])
        _analyzer.prepare(ctx_id=-1, det_size=(640, 640))
    return _analyzer


@dataclass
class Result:
    valid: bool = True
    reasons: list[str] = field(default_factory=list)
    metrics: dict = field(default_factory=dict)

    def fail(self, reason: str) -> "Result":
        self.valid = False
        self.reasons.append(reason)
        return self


def _eye_distance(kps: np.ndarray) -> float:
    """kps[0] = chap ko'z, kps[1] = o'ng ko'z (insightface tartibi)."""
    return float(np.linalg.norm(kps[0] - kps[1]))


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "model_loaded": _analyzer is not None}


@app.post("/validate")
async def validate(file: UploadFile = File(...)) -> dict:
    raw = await file.read()
    if not raw:
        return Result().fail("empty_file").__dict__
    if len(raw) > MAX_UPLOAD_BYTES:
        return Result().fail("file_too_large").__dict__

    try:
        # EXIF orientation'ni qo'llaymiz — telefon rasmlari aks holda yon yotadi.
        img = ImageOps.exif_transpose(Image.open(io.BytesIO(raw))).convert("RGB")
    except Exception:
        return Result().fail("not_an_image").__dict__

    result = Result()
    result.metrics["width"], result.metrics["height"] = img.size

    # insightface BGR kutadi
    bgr = np.asarray(img)[:, :, ::-1]
    faces = get_analyzer().get(bgr)

    if not faces:
        return result.fail("no_face_detected").__dict__
    if len(faces) > 1:
        result.fail("multiple_faces")
        result.metrics["face_count"] = len(faces)

    # Eng katta yuzni olamiz
    face = max(faces, key=lambda f: (f.bbox[2] - f.bbox[0]) * (f.bbox[3] - f.bbox[1]))

    eye_dist = _eye_distance(face.kps)
    result.metrics["eye_distance"] = round(eye_dist, 1)
    result.metrics["det_score"] = round(float(face.det_score), 3)
    if eye_dist < MIN_EYE_DISTANCE:
        result.fail(f"eyes_too_close:{eye_dist:.0f}<{MIN_EYE_DISTANCE}")

    # det_score 0..1 -> terminalning 0..100 "quality" shkalasiga yaqinlashtiramiz
    quality = float(face.det_score) * 100
    result.metrics["quality"] = round(quality, 1)
    if quality < MIN_QUALITY:
        result.fail(f"low_quality:{quality:.0f}<{MIN_QUALITY}")

    pose = getattr(face, "pose", None)
    if pose is not None:
        pitch, yaw, roll = (float(pose[0]), float(pose[1]), float(pose[2]))
        result.metrics |= {
            "pitch": round(pitch, 1),
            "yaw": round(yaw, 1),
            "roll": round(roll, 1),
        }
        if abs(yaw) > MAX_YAW:
            result.fail(f"head_turned:yaw={yaw:.0f}")
        if abs(pitch) > MAX_PITCH:
            result.fail(f"head_tilted:pitch={pitch:.0f}")
        if abs(roll) > MAX_ROLL:
            result.fail(f"head_rolled:roll={roll:.0f}")

    return result.__dict__
