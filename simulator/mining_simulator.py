import random, sys, os
import numpy as np
import pandas as pd
import matplotlib.pyplot as plt
from scipy.stats import nbinom, geom, pareto
from time import time
from datetime import datetime, timedelta
import ast
import copy
from pytz import timezone

SHOW_PROCESS = True # print current block number or not
SIM_START_TIME = datetime.now(timezone('Asia/Seoul')).strftime("%Y-%m-%d_%H-%M-%S.%f")[:-3]
GRAPH_PATH = 'graphs/' + SIM_START_TIME + '/'  # result graph image path
RESULT_DATA_PATH = 'results/' + SIM_START_TIME + '/' # result raw data path
PARTICIPANTS_MINING_POWERS_PATH = 'miningPowers/'



class MinerGroup:
    def __init__(self, start_time=0, end_time=0, mining_powers=[], start_times=[], mining_power_sums=[]):
        # mining start time
        self.start_time = start_time
        # mining end time
        self.end_time = end_time
        # mining powers of participants within this group
        self.mining_powers = mining_powers
        # to measure how many trials to succeed mining
        self.start_times = start_times
        self.mining_power_sums = mining_power_sums

    def add_miners(self, node_mining_prob, current_time, added_mining_powers):
        '''add new miners in this group'''

        if len(added_mining_powers) == 0:
            return

        # update mining_powers
        self.mining_powers += added_mining_powers

        # update end_time if needed
        trial_num = geom.rvs(node_mining_prob, size=1)[0]
        new_end_time = current_time + trial_num / sum(added_mining_powers)
        if self.end_time > new_end_time:
            # newly added miners find correct nonce faster than existing miners
            self.end_time = new_end_time
        
        # update start_times & mining_power_sums
        self.start_times.append(current_time)
        self.mining_power_sums.append(sum(added_mining_powers))

    def print_stats(self):
        '''print miner group's stats'''
        print("start_time:", self.start_time)
        print("end_time:", self.end_time)
        print("how many miners:", len(self.mining_powers))
        print("mining power sum:", sum(self.mining_powers))
        print("mining_powers:", self.mining_powers)



# generate random mining powers following pareto distribution
def gen_mining_powers_pareto(participants_num, total_mining_power, pareto_alpha, read_existing):

    if read_existing:
        try:
            # try to read existing one
            file_name = f"participant_mining_powers_pareto_{participants_num}_{total_mining_power}_{pareto_alpha}.xlsx"
            df_read = pd.read_excel(PARTICIPANTS_MINING_POWERS_PATH+file_name, engine='openpyxl')
            participants_mining_powers = df_read['participants_mining_powers'].dropna().tolist()
            return participants_mining_powers
        except:
            pass

    # generate new participant mining powers
    participants_mining_powers = pareto.rvs(pareto_alpha, size=participants_num)
    participants_mining_powers = [round(x * total_mining_power/sum(participants_mining_powers), 3) for x in participants_mining_powers] # normalize for fair comparison
    participants_mining_powers.sort(reverse=True)
    
    # save participant mining powers as xlsx file
    results = {}
    results['participants_mining_powers'] = participants_mining_powers
    df = pd.DataFrame(results)
    file_name = f"participant_mining_powers_pareto_{participants_num}_{total_mining_power}_{pareto_alpha}.xlsx"
    excel_writer = pd.ExcelWriter(PARTICIPANTS_MINING_POWERS_PATH+file_name, engine='openpyxl')
    # df.to_excel(excel_writer, sheet_name='Sheet1', index=False)
    df.to_excel(excel_writer, index=False)
    excel_writer.save()

    return participants_mining_powers



# split a list into approximately equal-sized sublists
def split_list(lst, n):
    k, m = divmod(len(lst), n)
    return [lst[i * k + min(i, m):(i + 1) * k + min(i + 1, m)] for i in range(n)]



# split input_list into len(target_list) lists
# then add each list to each target_list's element
# then target_list's elements have similar values
def split_list_greedy(target_list, input_list):
    partitions = [[element] for element in target_list]
    output_list = [[] for _ in range(len(target_list))]

    input_list.sort(reverse=True)
    for num in input_list:
        min_partition = min(partitions, key=sum)
        min_partition_index = partitions.index(min_partition)
        min_partition.append(num)
        output_list[min_partition_index].append(num)

    return output_list



# get min, q1, med, q3, max, avg, std of list
def get_list_stats(input_list):
    stats = [min(input_list), np.percentile(input_list, 25), 
            np.percentile(input_list, 50), np.percentile(input_list, 75), max(input_list),
            np.average(input_list), np.std(input_list)]
    stats = [round(stat, 3) for stat in stats]
    return stats



# print min, q1, med, q3, max, avg, std
def print_list_stats(stats, list_name):
    print(list_name, "stats\n  => min:", format(stats[0], ",.3f"), "/ q1:", format(stats[1], ",.3f"), "/ med:", format(stats[2], ",.3f"),
          "/ q3:", format(stats[3], ",.3f"), "/ max:", format(stats[4], ",.3f"),
          "( avg:", format(stats[5], ",.3f"), "/ std:", format(stats[6], ",.3f"), ")")



# do_optimal_mining_power_split: divide mining power as evenly as possible through greedy algorithm 
#   or naively divide mining power to have an equal number of participants in each miner group (about two times faster than the greedy algorithm)
#
# option 1) ETH with one mining pool: node_num_per_block = 1
#
# option 2) TH with one mining pool: node_num_per_block > 1, max_group_num = 1
#  -> all miners mine on one node at the same time (strategy with maximum broadcast cost)
#
# option 3) TH with one mining pool: node_num_per_block > 1, max_group_num > 1 (should be: node_num_per_block >> max_group_num)
#  -> efficient heuristic strategy: simultaneously mining as many nodes as possible with similar mining power
def simulate_mining_v1(block_num_to_mine, total_difficulty_prob, node_num_per_block, 
                       max_group_num, do_optimal_mining_power_split, participants_mining_powers, result_file_suffix):
    if block_num_to_mine == 0:
        return [], [], ""
    
    print("start simulation v1:", result_file_suffix)
    start_time = datetime.now()

    # set node's mining difficulty
    node_mining_prob = total_difficulty_prob*node_num_per_block
    process_print_interval = max(1, int(block_num_to_mine/1000))

    # final results
    block_mining_times = []
    block_mining_trial_nums = []
    total_node_mining_times = []
    total_node_mining_trial_nums = []
    broadcast_costs = []

    for block_num in range(block_num_to_mine):
        if SHOW_PROCESS and block_num % process_print_interval == 0 and block_num != 0:
            elapsed_time = datetime.now()-start_time
            remaining_time = timedelta(seconds=elapsed_time.total_seconds() / block_num * (block_num_to_mine-block_num))
            print("block num:", format(block_num, ","), "/", format(block_num_to_mine, ","), "(", round(block_num/block_num_to_mine*100, 3), 
                "% ) -> elapsed time:", elapsed_time, "/ estimated remaining time:", remaining_time, end="\r")

        # vars for this block
        current_time = 0
        broadcast_cost = 0
        node_mining_times = []
        node_mining_trial_nums = []
        current_group_num = min(max_group_num, node_num_per_block)
        checkpoints_to_decrease_miner_groups = random.sample(range(0, node_num_per_block-2), current_group_num-1) + [node_num_per_block]*2
        checkpoints_to_decrease_miner_groups.sort(reverse=True)
        # print("checkpoints:", checkpoints_to_decrease_miner_groups)

        # init miner groups
        miner_groups = []
        if do_optimal_mining_power_split:
            group_mining_powers = split_list_greedy([0]*current_group_num, participants_mining_powers)
            # print("group_mining_powers:", group_mining_powers)
        else:
            group_mining_powers = split_list(participants_mining_powers, current_group_num)
        for group_index in range(current_group_num):
            trial_num = geom.rvs(node_mining_prob, size=1)[0]
            group_mining_power = sum(group_mining_powers[group_index])
            miner_groups.append(MinerGroup(0, trial_num / group_mining_power, group_mining_powers[group_index], [0], [group_mining_power]))
            # miner_groups[group_index].print_stats()

        # process node mining
        for node_index in range(node_num_per_block):
            # print("  node ", node_num)

            # find which group will finish mining first
            current_group_num = len(miner_groups)
            fastest_group_index = 0
            min_end_time = miner_groups[0].end_time
            for group_index in range(current_group_num):
                if min_end_time > miner_groups[group_index].end_time:
                    fastest_group_index = group_index
                    min_end_time = miner_groups[group_index].end_time
            fastest_group = miner_groups[fastest_group_index]

            # deal with this group
            broadcast_cost += len(fastest_group.mining_powers) # broadcast to this group that someone has succeeded in mining
            node_mining_time = fastest_group.end_time - fastest_group.start_time
            node_mining_times.append(node_mining_time)
            actual_trial_num = 0
            for i in range(len(fastest_group.start_times)):
                actual_trial_num += (fastest_group.end_time - fastest_group.start_times[i]) * fastest_group.mining_power_sums[i]
            node_mining_trial_nums.append(actual_trial_num)
            current_time = fastest_group.end_time
            mined_node_num = node_index+1
            if mined_node_num <= checkpoints_to_decrease_miner_groups[current_group_num]:
                # assign next node to this group
                trial_num = geom.rvs(node_mining_prob, size=1)[0]
                fastest_group.start_time = current_time
                fastest_group.end_time = current_time + trial_num / sum(fastest_group.mining_powers)
                fastest_group.start_times = [current_time]
                fastest_group.mining_power_sums = [sum(fastest_group.mining_powers)]
            else:
                # merge this group with other groups
                if do_optimal_mining_power_split:
                    group_mining_powers = [sum(miner.mining_powers) for miner in miner_groups]
                    group_mining_powers[fastest_group_index] = sum(participants_mining_powers) + 1 # set max power not to add miner to this fastest group
                    splited_mining_powers = split_list_greedy(group_mining_powers, fastest_group.mining_powers)
                else:
                    splited_mining_powers = split_list(fastest_group.mining_powers, current_group_num-1) # TODO: use function to split participants_mining_powers
                    splited_mining_powers.insert(fastest_group_index, [])
                
                for group_index in range(current_group_num):
                    miner_groups[group_index].add_miners(node_mining_prob, current_time, splited_mining_powers[group_index])
                del miner_groups[fastest_group_index]



        # save block mining results
        total_node_mining_times.append(node_mining_times)
        total_node_mining_trial_nums.append(node_mining_trial_nums)
        block_mining_times.append(current_time)
        block_mining_trial_nums.append(sum(node_mining_trial_nums))
        broadcast_costs.append(broadcast_cost)
        # print block mining results
        # print("print block", block_num, "'s mining result")
        # print("  avg node mining time:", sum(node_mining_times)/len(node_mining_times))
        # print("  block mining time:", current_time)
        # print("  broadcast cost:", broadcast_cost)
        # print("\n")



    # print total results
    print("\n******************** SIM V1 RESULTS ********************")
    print("block_num_to_mine:", format(block_num_to_mine, ","))
    print("total_difficulty_prob:", total_difficulty_prob)
    print("node_num_per_block:", format(node_num_per_block, ","))
    print("node_mining_prob:", node_mining_prob)
    print("do_optimal_mining_power_split:", do_optimal_mining_power_split)
    print("max_group_num:", format(max_group_num, ","))
    print("participants num:", format(len(participants_mining_powers), ","))
    print("total mining power:", format(sum(participants_mining_powers), ",.0f"))
    print_list_stats(get_list_stats(participants_mining_powers), "participants mining powers")
    print("")

    total_node_mining_times_flattened = np.array(total_node_mining_times).flatten().tolist()
    print_list_stats(get_list_stats(total_node_mining_times_flattened), "node mining times")
    print("")
    total_node_mining_trial_nums_flattened = np.array(total_node_mining_trial_nums).flatten().tolist()
    print_list_stats(get_list_stats(total_node_mining_trial_nums_flattened), "node mining trial nums")
    print("")
    block_mining_times_stats = get_list_stats(block_mining_times)
    print_list_stats(block_mining_times_stats, "block mining times")
    print("")
    block_mining_trial_nums_stats = get_list_stats(block_mining_trial_nums)
    print_list_stats(block_mining_trial_nums_stats, "block mining trial nums")
    print("")
    broadcast_costs_stats = get_list_stats(broadcast_costs)
    print_list_stats(broadcast_costs_stats, "broadcast costs")
    print("********************************************************")



    # collect total results
    block_mining_times_copy = copy.deepcopy(block_mining_times) # copy this list since it should be returned without modification
    broadcast_costs_copy = copy.deepcopy(broadcast_costs) # copy this list since it should be returned without modification
    participants_mining_powers_copy = copy.deepcopy(participants_mining_powers) # copy this list since it should be reused in other functions
    results = {}
    results['block_num_to_mine'] = [block_num_to_mine]
    results['total_difficulty_prob'] = [total_difficulty_prob]
    results['node_num_per_block'] = [node_num_per_block]
    results['node_mining_prob'] = [node_mining_prob]
    results['max_group_num'] = [max_group_num]
    results['do_optimal_mining_power_split'] = [do_optimal_mining_power_split]
    results['participants_mining_powers'] = participants_mining_powers_copy

    results['block_mining_times'] = block_mining_times_copy
    results['block_mining_trial_nums'] = block_mining_trial_nums
    results['total_node_mining_times'] = total_node_mining_times
    results['total_node_mining_trial_nums'] = total_node_mining_trial_nums
    results['broadcast_costs'] = broadcast_costs_copy

    # find max length list
    maxLen = -1
    for k, v in results.items():
        listLen = len(results[k])
        if maxLen < listLen or maxLen == -1:
            maxLen = listLen

    # adjusting the lengths of the lists to be the same
    for k, v in results.items():
        results[k].extend([None] * (maxLen - len(v)))

    # save results as xlsx file
    df = pd.DataFrame(results)
    file_name = f"sim_result_v1_{result_file_suffix}.xlsx"
    excel_writer = pd.ExcelWriter(RESULT_DATA_PATH+file_name, engine='openpyxl')
    # df.to_excel(excel_writer, sheet_name='Sheet1', index=False)
    df.to_excel(excel_writer, index=False)
    excel_writer.save()



    end_time = datetime.now()
    print("\nsimulation execution time:", end_time-start_time)
    print("  => result file:", RESULT_DATA_PATH, file_name)
    print("\n\n\n\n")
    
    return block_mining_times, broadcast_costs, file_name



def analyze_mining_simulation_results_v1(file_name):
    if file_name == "":
        return

    # read xlsx file
    df_read = pd.read_excel(RESULT_DATA_PATH+file_name, engine='openpyxl')

    # parse columns
    block_num_to_mine = int(df_read['block_num_to_mine'].dropna().tolist()[0])
    total_difficulty_prob = df_read['total_difficulty_prob'].dropna().tolist()[0]
    node_num_per_block = int(df_read['node_num_per_block'].dropna().tolist()[0])
    node_mining_prob = df_read['node_mining_prob'].dropna().tolist()[0]
    max_group_num = int(df_read['max_group_num'].dropna().tolist()[0])
    do_optimal_mining_power_split = bool(int(df_read['do_optimal_mining_power_split'].dropna().tolist()[0]))
    participants_mining_powers = df_read['participants_mining_powers'].dropna().tolist()
    block_mining_times = df_read['block_mining_times'].dropna().tolist()
    block_mining_trial_nums = df_read['block_mining_trial_nums'].dropna().tolist()
    total_node_mining_times = df_read['total_node_mining_times'].dropna().tolist()
    total_node_mining_trial_nums = df_read['total_node_mining_trial_nums'].dropna().tolist()
    broadcast_costs = df_read['broadcast_costs'].dropna().tolist()

    # convert str into list
    for i in range(len(total_node_mining_times)):
        total_node_mining_times[i] = ast.literal_eval(total_node_mining_times[i])
        total_node_mining_trial_nums[i] = ast.literal_eval(total_node_mining_trial_nums[i])
    
    # print total results
    print("\n\n******************** SIM V1 RESULTS ********************")
    print("block_num_to_mine:", format(block_num_to_mine, ","))
    print("total_difficulty_prob:", total_difficulty_prob)
    print("node_num_per_block:", format(node_num_per_block, ","))
    print("node_mining_prob:", node_mining_prob)
    print("do_optimal_mining_power_split:", do_optimal_mining_power_split)
    print("max_group_num:", format(max_group_num, ","))
    print("participants num:", format(len(participants_mining_powers), ","))
    print("total mining power:", format(sum(participants_mining_powers), ",.0f"))
    print_list_stats(get_list_stats(participants_mining_powers), "participants mining powers")
    print("")

    total_node_mining_times_flattened = np.array(total_node_mining_times).flatten().tolist()
    print_list_stats(get_list_stats(total_node_mining_times_flattened), "node mining times")
    print("")
    total_node_mining_trial_nums_flattened = np.array(total_node_mining_trial_nums).flatten().tolist()
    print_list_stats(get_list_stats(total_node_mining_trial_nums_flattened), "node mining trial nums")
    print("")
    block_mining_times_stats = get_list_stats(block_mining_times)
    print_list_stats(block_mining_times_stats, "block mining times")
    print("")
    block_mining_trial_nums_stats = get_list_stats(block_mining_trial_nums)
    print_list_stats(block_mining_trial_nums_stats, "block mining trial nums")
    print("")
    broadcast_costs_stats = get_list_stats(broadcast_costs)
    print_list_stats(broadcast_costs_stats, "broadcast costs")
    print("********************************************************")
    print("\n\n\n\n")



# recommanded params
#   block_num_to_mine > 1,000,000
#   total_difficulty_prob = 0.0000000065
#   total_mining_power = 10,000,000
#   pareto_alpha = 5
#
# option 1) TH with one mining pool: node_num_per_block > 1, do_broadcast_per_node = True
#   -> all miners mine on one node at the same time (strategy with maximum broadcast cost)
#
# option 2) ETH with one mining pool: node_num_per_block = 1, do_broadcast_per_node = True 
#   -> almost same results as 'ETH with multiple miners', but cannot measure win_counts
#
# option 3) TH with independent miners: node_num_per_block > 1, do_broadcast_per_node = False 
#   -> miners cannot receive rewards proportional to their mining power
#   this option can be seen as a stupid mining pool
#   so even if we retain only the most powerful miner, the outcome remains unchanged (i.e., weak participants are useless)
#
# option 4) ETH with independent miners: node_num_per_block = 1, do_broadcast_per_node = False
#   -> miners receive rewards proportional to their mining power
def simulate_mining_v2(block_num_to_mine, total_difficulty_prob, node_num_per_block, 
                       do_broadcast_per_node, participants_mining_powers, result_file_suffix):
    if block_num_to_mine == 0:
        return [], ""
    
    print("start simulation v2:", result_file_suffix)
    start_time = datetime.now()

    # set node's mining difficulty
    node_mining_prob = total_difficulty_prob*node_num_per_block
    process_print_interval = max(1, int(block_num_to_mine/1000))

    participants_mining_powers.sort()

    # final results
    block_mining_times = []
    block_mining_trial_nums = []
    win_counts = [0]*len(participants_mining_powers)
    broadcast_costs = []
    print("start block mining")
    for block_num in range(block_num_to_mine):
        if SHOW_PROCESS and block_num % process_print_interval == 0 and block_num != 0:
            elapsed_time = datetime.now()-start_time
            remaining_time = timedelta(seconds=elapsed_time.total_seconds() / block_num * (block_num_to_mine-block_num))
            print("block num:", format(block_num, ","), "/", format(block_num_to_mine, ","), "(", round(block_num/block_num_to_mine*100, 3), 
                "% ) -> elapsed time:", elapsed_time, "/ estimated remaining time:", remaining_time, end="\r")

        if do_broadcast_per_node:
            # a mining pool with 'transmit-after-mining-each-trie-nodes' strategy
            total_mining_power = sum(participants_mining_powers)
            trial_num = nbinom.rvs(node_num_per_block, node_mining_prob, size=1)[0] + node_num_per_block
            block_mining_time = trial_num / total_mining_power
            block_mining_trial_nums.append(trial_num)
            block_mining_times.append(block_mining_time)
            broadcast_costs.append(len(participants_mining_powers)*node_num_per_block)
        else:
            # independent miners (or a mining pool with 'transmit-after-mining-all-trie-nodes' strategy)
            trial_nums = nbinom.rvs(node_num_per_block, node_mining_prob, size=len(participants_mining_powers)) + node_num_per_block
            participant_mining_times = [trial_num / mining_power for trial_num, mining_power in zip(trial_nums, participants_mining_powers)]
            winner_participant_index = participant_mining_times.index(min(participant_mining_times))
            block_mining_times.append(participant_mining_times[winner_participant_index])
            block_mining_trial_nums.append(trial_nums[winner_participant_index])
            win_counts[winner_participant_index] += 1
            broadcast_costs.append(len(participants_mining_powers))



    # print final results
    print("\n******************** SIM V2 RESULTS ********************")
    print("block_num_to_mine:", format(block_num_to_mine, ","))
    print("total_difficulty_prob:", total_difficulty_prob)
    print("node_num_per_block:", format(node_num_per_block, ","))
    print("node_mining_prob:", node_mining_prob)
    print("do_broadcast_per_node:", do_broadcast_per_node)
    print("participants num:", format(len(participants_mining_powers), ","))
    print("total mining power:", format(sum(participants_mining_powers), ",.0f"))
    print_list_stats(get_list_stats(participants_mining_powers), "participants mining powers")
    print("")

    block_mining_times_stats = get_list_stats(block_mining_times)
    block_mining_trial_nums_stats = get_list_stats(block_mining_trial_nums)
    print_list_stats(block_mining_times_stats, "block mining times")
    print("")
    print_list_stats(block_mining_trial_nums_stats, "block mining trial nums")
    print("")
    
    # win_counts was measured, analyze it
    if not do_broadcast_per_node:
        reward_per_unit_mining_power = [win_count / mining_power for win_count, mining_power in zip(win_counts, participants_mining_powers)]
        avg_reward = sum(reward_per_unit_mining_power)/len(reward_per_unit_mining_power)
        reward_per_unit_mining_power = [round(reward/avg_reward, 3) for reward in reward_per_unit_mining_power] # normalize
        print_list_stats(get_list_stats(reward_per_unit_mining_power), "reward per unit mining power (avg is normalized to 1, thus inequality occurs when the min and max values deviate significantly from 1)")
        
        top_winner_num_to_print = min(10, len(reward_per_unit_mining_power))
        reward_per_unit_mining_power.sort()
        print("  => top winners' rewards:", reward_per_unit_mining_power[-top_winner_num_to_print:])
    
    print("")
    print("avg broadcast cost:", format(np.average(broadcast_costs), ",.3f"))
    print("********************************************************")



    # collect total results
    participants_mining_powers_copy = copy.deepcopy(participants_mining_powers) # copy this list since it should be reused in other functions
    win_counts_copy = copy.deepcopy(win_counts) # copy this list since it should be returned without modification
    results = {}
    results['block_num_to_mine'] = [block_num_to_mine]
    results['total_difficulty_prob'] = [total_difficulty_prob]
    results['node_num_per_block'] = [node_num_per_block]
    results['node_mining_prob'] = [node_mining_prob]
    results['do_broadcast_per_node'] = [do_broadcast_per_node]
    results['participants_mining_powers'] = participants_mining_powers_copy
    results['block_mining_times_stats'] = block_mining_times_stats
    results['block_mining_trial_nums_stats'] = block_mining_trial_nums_stats
    results['win_counts'] = win_counts_copy
    results['broadcast_costs'] = [np.average(broadcast_costs)]

    # find max length list
    maxLen = -1
    for k, v in results.items():
        listLen = len(results[k])
        if maxLen < listLen or maxLen == -1:
            maxLen = listLen

    # adjusting the lengths of the lists to be the same
    for k, v in results.items():
        results[k].extend([None] * (maxLen - len(v)))

    # save results as xlsx file
    df = pd.DataFrame(results)
    file_name = f"sim_result_v2_{result_file_suffix}.xlsx"
    excel_writer = pd.ExcelWriter(RESULT_DATA_PATH+file_name, engine='openpyxl')
    # df.to_excel(excel_writer, sheet_name='Sheet1', index=False)
    df.to_excel(excel_writer, index=False)
    excel_writer.save()



    end_time = datetime.now()
    print("\nsimulation execution time:", end_time-start_time)
    print("  => result file:", RESULT_DATA_PATH, file_name)
    print("\n\n\n\n")

    return win_counts, file_name



def analyze_mining_simulation_results_v2(file_name):
    if file_name == "":
        return

    # read xlsx file
    df_read = pd.read_excel(RESULT_DATA_PATH+file_name, engine='openpyxl')

    # parse columns
    block_num_to_mine = int(df_read['block_num_to_mine'].dropna().tolist()[0])
    total_difficulty_prob = df_read['total_difficulty_prob'].dropna().tolist()[0]
    node_num_per_block = int(df_read['node_num_per_block'].dropna().tolist()[0])
    node_mining_prob = df_read['node_mining_prob'].dropna().tolist()[0]
    do_broadcast_per_node = bool(int(df_read['do_broadcast_per_node'].dropna().tolist()[0]))
    participants_mining_powers = df_read['participants_mining_powers'].dropna().tolist()
    block_mining_times_stats = df_read['block_mining_times_stats'].dropna().tolist()
    block_mining_trial_nums_stats = df_read['block_mining_trial_nums_stats'].dropna().tolist()
    win_counts = df_read['win_counts'].dropna().tolist()
    broadcast_costs = df_read['broadcast_costs'].dropna().tolist()

    # print final results
    print("\n\n******************** SIM V2 RESULTS ********************")
    print("block_num_to_mine:", format(block_num_to_mine, ","))
    print("total_difficulty_prob:", total_difficulty_prob)
    print("node_num_per_block:", format(node_num_per_block, ","))
    print("node_mining_prob:", node_mining_prob)
    print("do_broadcast_per_node:", do_broadcast_per_node)
    print("participants num:", format(len(participants_mining_powers), ","))
    print("total mining power:", format(sum(participants_mining_powers), ",.0f"))
    print_list_stats(get_list_stats(participants_mining_powers), "participants mining powers")
    print("")

    print_list_stats(block_mining_times_stats, "block mining times")
    print("")
    print_list_stats(block_mining_trial_nums_stats, "block mining trial nums")
    print("")

    # win_counts was measured, analyze it
    if not do_broadcast_per_node:
        reward_per_unit_mining_power = [win_count / mining_power for win_count, mining_power in zip(win_counts, participants_mining_powers)]
        avg_reward = sum(reward_per_unit_mining_power)/len(reward_per_unit_mining_power)
        reward_per_unit_mining_power = [round(reward/avg_reward, 3) for reward in reward_per_unit_mining_power] # normalize
        print_list_stats(get_list_stats(reward_per_unit_mining_power), "reward per unit mining power (avg is normalized to 1, thus inequality occurs when the min and max values deviate significantly from 1)")

        top_winner_num_to_print = min(10, len(reward_per_unit_mining_power))
        reward_per_unit_mining_power.sort()
        print("  => top winners' rewards:", reward_per_unit_mining_power[-top_winner_num_to_print:])

    print("")
    print("avg broadcast cost:", format(sum(broadcast_costs)/len(broadcast_costs), ",.3f"))
    print("********************************************************")
    print("\n\n\n\n")




def draw_box_plot_mining_time(bmt_lists, labels, graph_name_suffix):

    plt.figure()
    plt.title("block mining times")
    plt.xlabel('type of mining pool')
    plt.ylabel('sec')
    plt.boxplot(bmt_lists, whis = 1.5)
    idx = range(1, 1+len(labels))
    plt.xticks(idx, labels)
    plt.savefig(GRAPH_PATH + "blockMiningTimes_" + graph_name_suffix + ".png")



def draw_box_plot_broadcast_cost(bmt_lists, labels, graph_name_suffix):

    plt.figure()
    fig, ax = plt.subplots()
    plt.title("broadcast cost per block")
    plt.xlabel('type of mining pool (with percentage of # of leaf nodes)')
    plt.ylabel('# of participants')
    idx = range(1, 1+len(labels))
    bp = ax.boxplot(bmt_lists, whis = 1.5, positions=idx)
    plt.xticks(idx, labels)

    # show avgs & stds in graph
    # avgs = []
    # stds = []
    # for bc in bc_th_greedy + [bc_eth]:
    #     avgs.append(sum(bc)/len(bc))
    #     stds.append(np.std(bc))
    # for i, line in enumerate(bp['medians']):
    #     x, y = line.get_xydata()[1]
    #     text = ' avg={:.0f}\n std={:.0f}'.format(avgs[i], stds[i])
    #     ax.annotate(text, xy=(x, y))
    # print("avgs:", avgs)
    # print("stds:", stds)

    plt.savefig(GRAPH_PATH + "broadcastCost_" + graph_name_suffix + ".png")



def draw_plot_win_count(win_counts, participants_mining_powers, graph_name_suffix):

    # win_counts was measured, analyze it
    if sum(win_counts) != 0:
        # draw graph
        # create figure and axis objects with subplots()
        fig,ax = plt.subplots()
        # ax.plot(range(len(participants_mining_powers)), participants_mining_powers, color="red", marker="o", markersize=4)
        ax.scatter(range(len(participants_mining_powers)), participants_mining_powers, color="red", s=9)
        ax.set_xlabel("participant index (weak <-> strong)", fontsize = 14)
        ax.set_ylabel("mining power", color="red", fontsize=14)
        ax.ticklabel_format(style='sci', axis='y', scilimits=(0,0))

        # twin object for two different y-axis on the sample plot
        ax2=ax.twinx()
        # ax2.plot(range(len(participants_mining_powers)), win_counts, color="blue", marker="o", markersize=2)
        ax2.scatter(range(len(participants_mining_powers)), win_counts, color="blue", s=3)
        ax2.set_ylabel("# of mined blocks", color="blue", fontsize=14)
        ax2.ticklabel_format(style='sci', axis='y', scilimits=(0,0))

        fig.savefig(GRAPH_PATH + "miningPowersAndRewards_" + graph_name_suffix + ".png")





if __name__ == '__main__':

    start_time = datetime.now()

    # make dirs for simulation results
    os.makedirs(GRAPH_PATH, exist_ok = True)
    os.makedirs(RESULT_DATA_PATH, exist_ok = True)
    os.makedirs(PARTICIPANTS_MINING_POWERS_PATH, exist_ok = True)

    #
    # set simulation params
    #

    # SHOW_PROCESS = False
    # bntm: block num to mine
    bntm_for_eth_mining_times = 100000 # recommand: 100000
    bntm_for_th_broadcast_costs = 1000 # recommand: 1000
    bntm_for_mining_rewards = 5000000 # recommand: for 100 participants: 1000000 / for 1,000 participants: 5000000
    # nnpb: node num per block
    nnpb_for_th = 1000
    # set how many leaf nodes in TH
    max_group_nums = [1, int(nnpb_for_th*0.1), int(nnpb_for_th*0.2), int(nnpb_for_th*0.5), nnpb_for_th-1]
    th_labels = ['TH_1', 'TH_10', 'TH_20', 'TH_50', 'TH_99']
    # total difficulty probability
    tdp = 0.0000000065
    # generate random mining powers
    participants_mining_powers = gen_mining_powers_pareto(participants_num = 1000,
                                                          total_mining_power = 10000000,
                                                          pareto_alpha = 5,
                                                          read_existing = True)



    #
    # for simulation v1
    #

    # option 1) ETH with one mining pool: node_num_per_block = 1
    #  -> ETH's block mining time shows a large deviation unlike TH's
    bmt_eth, bc_eth, file_name = simulate_mining_v1(block_num_to_mine = bntm_for_eth_mining_times,
                       total_difficulty_prob= tdp,
                       node_num_per_block = 1,
                       max_group_num = 1,
                       do_optimal_mining_power_split = False,
                       participants_mining_powers = participants_mining_powers,
                       result_file_suffix = 'ETH')
    # analyze_mining_simulation_results_v1(file_name)

    # option 2) TH with one mining pool: node_num_per_block > 1, max_group_num = 1
    #  -> all miners mine on one node at the same time (strategy with maximum broadcast cost)
    # option 3) TH with one mining pool: node_num_per_block > 1, max_group_num > 1 (should be: node_num_per_block >> max_group_num)
    #  -> efficient heuristic strategy: simultaneously mining as many nodes as possible with similar mining power
    bmt_th_greedy = []
    bc_th_greedy = []
    for index, mgn in enumerate(max_group_nums):
        bmt, bc, file_name = simulate_mining_v1(block_num_to_mine = bntm_for_th_broadcast_costs,
                       total_difficulty_prob= tdp,
                       node_num_per_block = nnpb_for_th,
                       max_group_num = mgn,
                       do_optimal_mining_power_split = True,
                       participants_mining_powers = participants_mining_powers,
                       result_file_suffix = th_labels[index]+'_greedy')
        # analyze_mining_simulation_results_v1(file_name)
        bmt_th_greedy.append(bmt)
        bc_th_greedy.append(bc)
    
    bmt_th_naive = []
    bc_th_naive = []
    for index, mgn in enumerate(max_group_nums):
        bmt, bc, file_name = simulate_mining_v1(block_num_to_mine = bntm_for_th_broadcast_costs,
                       total_difficulty_prob= tdp,
                       node_num_per_block = nnpb_for_th,
                       max_group_num = mgn,
                       do_optimal_mining_power_split = False,
                       participants_mining_powers = participants_mining_powers,
                       result_file_suffix = th_labels[index]+'_naive')
        # analyze_mining_simulation_results_v1(file_name)
        bmt_th_naive.append(bmt)
        bc_th_naive.append(bc)

    # draw box plots for block mining times
    if bntm_for_eth_mining_times != 0:
        draw_box_plot_mining_time([bmt_eth], ['ETH'], 'ETH')
    if bntm_for_th_broadcast_costs != 0:
        draw_box_plot_mining_time(bmt_th_greedy, th_labels, 'TH_greedy')
        draw_box_plot_mining_time(bmt_th_naive, th_labels, 'TH_naive')

    # draw box plots for broadcast costs
    if bntm_for_th_broadcast_costs != 0:
        if bntm_for_eth_mining_times == 0:
            bc_eth = [len(participants_mining_powers)]
        draw_box_plot_broadcast_cost(bc_th_greedy[1:] + [bc_eth], th_labels[1:] + ['ETH'], 'greedy')
        draw_box_plot_broadcast_cost(bc_th_naive[1:] + [bc_eth], th_labels[1:] + ['ETH'], 'naive')



    #
    # for simulation v2
    #

    # option 1) ETH with independent miners: node_num_per_block = 1, do_broadcast_per_node = False
    #   -> miners receive rewards proportional to their mining power
    wc_eth, file_name = simulate_mining_v2(block_num_to_mine = bntm_for_mining_rewards,
                                           total_difficulty_prob = tdp,
                                           node_num_per_block = 1,
                                           do_broadcast_per_node = False,
                                           participants_mining_powers = participants_mining_powers,
                                           result_file_suffix = 'ETH')
    # analyze_mining_simulation_results_v2(file_name)

    # option 2) TH with independent miners: node_num_per_block > 1, do_broadcast_per_node = False
    #   -> miners cannot receive rewards proportional to their mining power
    #   this option can be seen as a stupid mining pool
    #   so even if we retain only the most powerful miner, the outcome remains unchanged (i.e., weak participants are useless)
    wc_th, file_name = simulate_mining_v2(block_num_to_mine = bntm_for_mining_rewards,
                                          total_difficulty_prob = tdp,
                                          node_num_per_block = nnpb_for_th,
                                          do_broadcast_per_node = False,
                                          participants_mining_powers = participants_mining_powers,
                                          result_file_suffix = 'TH')
    # analyze_mining_simulation_results_v2(file_name)
    
    # option 3) to prove stupid TH mining pool's inefficiency: show same result with only one miner
    wc, file_name = simulate_mining_v2(block_num_to_mine = 100000,
                                       total_difficulty_prob = tdp,
                                       node_num_per_block = nnpb_for_th,
                                       do_broadcast_per_node = False,
                                       participants_mining_powers = [max(participants_mining_powers)],
                                       result_file_suffix = 'TH_solo')
    # analyze_mining_simulation_results_v2(file_name)

    # draw plots for win counts
    if bntm_for_mining_rewards != 0:
        draw_plot_win_count(wc_eth, participants_mining_powers, 'ETH')
        draw_plot_win_count(wc_th, participants_mining_powers, 'TH')



    print('\nend simulation, total elapsed time:', datetime.now()-start_time)
    print("result dirs:", RESULT_DATA_PATH, "&", GRAPH_PATH)
