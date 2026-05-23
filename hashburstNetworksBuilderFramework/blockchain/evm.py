# File: hashburstNetworksBuilderFramework/blockchain/evm.py
class HashburstEVM:
    def __init__(self):
        self.contracts = {}

    def deploy_contract(self, contract_name: str, bytecode: str):
        self.contracts[contract_name] = bytecode
        return f"Contract {contract_name} deployed"

    def call_contract(self, contract_name: str, function_name: str, *args):
        # Simplified representation of invoking a contract
        return f"Called {function_name} on {contract_name} with arguments {args}"
