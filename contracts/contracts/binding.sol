// SPDX-License-Identifier: MIT

// This file is the sole input file we use when running abigen to generate the bindings in shuttermint/contract/binding.go
pragma solidity >=0.7.0 <0.8.0;

import "./BatcherContract.sol";
import "./ConfigContract.sol";
import "./DepositContract.sol";
import "./ExecutorContract.sol";
import "./FeeBankContract.sol";
import "./KeyBroadcastContract.sol";
import "./KeyperSlasher.sol";
import "./TestDepositTokenContract.sol";

import "./MockBatcherContract.sol";
import "./MockTargetContract.sol";
