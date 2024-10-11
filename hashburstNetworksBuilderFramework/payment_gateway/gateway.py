# File: ./payment_gateway/gateway.py
class PaymentGateway:
    def __init__(self):
        self.transactions = []

    def create_transaction(self, from_address, to_address, amount):
        transaction = {
            'from': from_address,
            'to': to_address,
            'amount': amount
        }
        self.transactions.append(transaction)
        return transaction
