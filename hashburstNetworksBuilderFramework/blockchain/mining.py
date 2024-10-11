# File: /blockchain/mining.py
import hashlib
import random
import time

class HashburstMining:
    def __init__(self, difficulty: int):
        self.difficulty = difficulty

    def mine_block(self, data: str) -> dict:
        nonce = 0
        start_time = time.time()
        prefix = '0' * self.difficulty

        while True:
            hash_result = hashlib.sha256(f'{data}{nonce}'.encode()).hexdigest()
            if hash_result.startswith(prefix):
                end_time = time.time()
                return {
                    'block_data': data,
                    'nonce': nonce,
                    'hash': hash_result,
                    'time_taken': end_time - start_time
                }
            nonce += 1
