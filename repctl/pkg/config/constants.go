/*
 Copyright © 2021-2022 Dell Inc. or its subsidiaries. All Rights Reserved.

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at
      http://www.apache.org/licenses/LICENSE-2.0
 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package config

// Config parameters for viper
// All the values are in lower case as viper is case insensitive for most parts
const (
	ReplicationPrefix            = "replication-config"
	Clusters                     = "clusters"
	Driver                       = "driver"
	ReplicationGroup             = "rg"
	ActionFailoverRemote         = "FAILOVER_REMOTE"
	ActionFailoverLocalUnplanned = "UNPLANNED_FAILOVER_LOCAL"
	ActionFailbackLocal          = "FAILBACK_LOCAL"
	ActionFailbackLocalDiscard   = "ACTION_FAILBACK_DISCARD_CHANGES_LOCAL"
	ActionReprotect              = "REPROTECT_LOCAL"
	ActionSwap                   = "SWAP_LOCAL"
	ActionSuspend                = "SUSPEND"
	ActionResume                 = "RESUME"
	ActionSync                   = "SYNC"
	ActionCreateSnapshot         = "CREATE_SNAPSHOT"
	Verbose                      = "verbose"
)
