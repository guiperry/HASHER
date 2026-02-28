import json
import os

def create_frame(source, chunk, start, tokens, target, slots):
    return {
        "source_file": source,
        "chunk_id": chunk,
        "window_start": start,
        "token_sequence": tokens,
        "target_token": target,
        "feature_vector": slots,
        "context_hash": 0
    }

# Token Mappings (cl100k_base)
# Hello: 9906
#  world: 1917
# world: 14957
# !: 0
# What: 3923
#  is: 374
#  your: 701
#  name: 836
# ?: 30
# Whats: 59175
# Hasher: [6504, 261]
# hasher: [8460, 261]

frames = []

# Basic patterns
frames.append(create_frame("demo.txt", 1, 0, [9906], 1917, [0]*12))
frames.append(create_frame("demo.txt", 2, 0, [1917], 0, [0]*12))
frames.append(create_frame("demo.txt", 3, 0, [14957], 0, [0]*12))

# Variations
frames.append(create_frame("demo.txt", 4, 0, [6504, 261], 1917, [0]*12))
frames.append(create_frame("demo.txt", 5, 0, [8460, 261], 1917, [0]*12))

# Questions
frames.append(create_frame("demo.txt", 6, 0, [3923], 374, [0]*12))
frames.append(create_frame("demo.txt", 7, 0, [374], 701, [0]*12))
frames.append(create_frame("demo.txt", 8, 0, [701], 836, [0]*12))
frames.append(create_frame("demo.txt", 9, 0, [836], 30, [0]*12))
frames.append(create_frame("demo.txt", 10, 0, [59175], 701, [0]*12))

out_dir = os.path.expanduser("~/.local/share/hasher/data/frames")
os.makedirs(out_dir, exist_ok=True)
out_path = os.path.join(out_dir, "training_frames.json")
with open(out_path, "w") as f:
    json.dump(frames, f, indent=2)

print(f"Generated {len(frames)} robust demo frames in {out_path}")
