# File: ./blockchain/minting.py
class HashburstMinting:
    def __init__(self, initial_supply=1000000):
        self.supply = initial_supply

    def mint_tokens(self, amount: int):
        self.supply += amount
        return self.supply
