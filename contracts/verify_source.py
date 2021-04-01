#! /usr/bin/env python3
"""Verify contract source code on etherscan

Make sure to set ETHERSCAN_TOKEN and WEB3_INFURA_PROJECT_ID
"""

import brownie.project as project
from brownie.network.main import connect

contracts = {
    "ConfigContract": "0xb9ce8a9a1529dE015025cEd08bC7BD56bA37Fd98",
    "KeyBroadcastContract": "0x4a8fb79Ef7503d460F5f00ea77f908F5F7EeF774",
    "FeeBankContract": "0xA763d64206c1b6F953Ed3446984c79FB99e6d42A",
    "BatcherContract": "0x515965E469514F795a3Bcbf8Cd774441e65f2d2a",
    "ExecutorContract": "0xFACB9A5Df90208ed2e1AF08A0D27ad16C98a9899",
    "TokenContract": "0x1C0B92EB7ebb3b66C88537E0f75d5f9EcF0F1E28",
    "DepositContract": "0xD26bf9a2ee542014074C0FDBd243d068f8213f77",
    "KeyperSlasherContract": "0x4f8b4af15cB9638E260C9DBCe1Cd9a57E10Bd94f",
    "TargetContract": "0xaCb6B87E4421dCaD69F19633e1C5AF3E004580Ec",
}


def main():
    project.load(".", name="shielder")
    connect(network="goerli", launch_rpc=False)
    import brownie.project.shielder

    for name, addr in contracts.items():
        try:
            cls = getattr(brownie.project.shielder, name)
        except AttributeError:
            print(f"Cannot find contract {name}")
            continue
        c = cls.at(addr)
        print(f"======> Verify {name}")
        try:
            cls.publish_source(c)
        except ValueError as e:
            print(f"{e}")


if __name__ == "__main__":
    main()
