# File: ./asset_generation/altcoin_mining.py
class AltcoinMining:
    def __init__(self, coin_name: str):
        self.coin_name = coin_name

    def mine_coin(self):
        print(f"Mining {self.coin_name}... Done!")
        return True
