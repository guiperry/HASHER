import json
import os

def create_frame(source, chunk, start, tokens, target, slots):
    # Map target token to vocab size 1000
    vocab_target = target % 1000
    return {
        "source_file": source,
        "chunk_id": chunk,
        "window_start": start,
        "token_sequence": tokens,
        "target_token": vocab_target,
        "feature_vector": slots,
        "context_hash": 0
    }

# Token Mappings (cl100k_base)
# Hello: 9906 -> 906
#  world: 1917 -> 917
# !: 0 -> 0
# What is: [3923, 374] -> [923, 374]
#  your: 701 -> 701
#  name: 836 -> 836
# ?: 30 -> 30

frames = []

# Pattern 1: Hello ->  world -> !
# [906] -> 917
frames.append(create_frame("demo.txt", 1, 0, [906], 917, [0x11111111, 0x22222222, 0x33333333, 0x44444444, 0x00000001, 0, 0, 0, 0, 0, 0x1000, 0]))
# [906, 917] -> 0
frames.append(create_frame("demo.txt", 1, 0, [906, 917], 0, [0x11111111, 0x22222222, 0x33333333, 0x44444444, 0x00000002, 0, 0, 0, 0, 0, 0x1000, 1]))

# Pattern 2: What is ->  your ->  name -> ?
# [923, 374] -> 701
frames.append(create_frame("demo.txt", 2, 0, [923, 374], 701, [0xAAAAAAAA, 0xBBBBBBBB, 0xCCCCCCCC, 0xDDDDDDDD, 0x00000001, 0, 0, 0, 0, 1, 0x1000, 0]))
# [923, 374, 701] -> 836
frames.append(create_frame("demo.txt", 2, 0, [923, 374, 701], 836, [0xAAAAAAAA, 0xBBBBBBBB, 0xCCCCCCCC, 0xDDDDDDDD, 0x00000002, 0, 0, 0, 0, 1, 0x1000, 1]))
# [923, 374, 701, 836] -> 30
frames.append(create_frame("demo.txt", 2, 0, [923, 374, 701, 836], 30, [0xAAAAAAAA, 0xBBBBBBBB, 0xCCCCCCCC, 0xDDDDDDDD, 0x00000003, 0, 0, 0, 0, 1, 0x1000, 2]))

out_dir = os.path.expanduser("~/.local/share/hasher/data/frames")
os.makedirs(out_dir, exist_ok=True)
out_path = os.path.join(out_dir, "training_frames.json")
with open(out_path, "w") as f:
    json.dump(frames, f, indent=2)

print(f"Generated {len(frames)} demo frames in {out_path}")
