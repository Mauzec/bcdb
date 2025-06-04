import matplotlib.pyplot as plt

plt.style.use('seaborn-v0_8-darkgrid')

lat = [123.393271,383.285463,301.800395,310.348577,315.4075,
    343.081597,312.701977,378.671791,395.70084,302.75321,
    310.421,338.821303,350.164212,310.77975,
    394.191789,353.653292,336.113327,325.113341,300.932112,
    353.427951,388.393776,377.907071,312.439214,355.439214,
    450.12341,338.70762,367.640503,348.393725,521.838569]
throught = [
    9553.56,7516.23,7210.23,7531.64,7547.30,8520.80,7251.02,7597.24,
    7257.96,7215.21,8430.12,7214.99,6752.122,7455.67,7542.67,7222.71,
    7322.71,7302.52,8404.52,7160.65,7169.42,7318.27,7133.02,7300.08,
    6173.10,6273.10,6309.91,5160.48,5512.63,6312.87,6300.42
]

nodes_lat = list(range(12, 12 + 2 * len(lat), 2))
nodes_throught = list(range(12, 12 + 2 * len(throught), 2))

fig, axs = plt.subplots(1, 2, figsize=(14, 6))

axs[0].plot(nodes_lat, lat, marker='D', color='#1f77b4', linewidth=2, label='Latency')
axs[0].set_title('Latency(ms)', fontsize=16)
axs[0].set_xlabel('Nodes', fontsize=13)
axs[0].set_ylabel('Latency', fontsize=13)
axs[0].legend()
axs[0].grid(True, linestyle='--', alpha=0.7)

axs[1].plot(nodes_throught, throught, marker='s', color='#ff7f0e', linewidth=2, label='Throughput')
axs[1].set_title('Throughput(req/s)', fontsize=16)
axs[1].set_xlabel('Nodes', fontsize=13)
axs[1].set_ylabel('Throughput', fontsize=13)
axs[1].legend()
axs[1].grid(True, linestyle='--', alpha=0.7)

plt.tight_layout()
plt.show()

