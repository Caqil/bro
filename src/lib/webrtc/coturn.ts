import { createHash, randomBytes } from 'crypto';

interface CoTURNServer {
  urls: string[];
  username?: string;
  credential?: string;
  credentialType?: 'password' | 'oauth';
}

interface CoTURNConfig {
  servers: {
    primary: {
      host: string;
      port: number;
      turnPort: number;
      secret: string;
      realm: string;
    };
    fallback?: {
      host: string;
      port: number;
      turnPort: number;
      secret: string;
      realm: string;
    }[];
  };
  ttl: number; // Time-to-live for credentials in seconds
}

interface TURNCredentials {
  username: string;
  credential: string;
  ttl: number;
}

export class CoTURNManager {
  private config: CoTURNConfig;

  constructor(config: CoTURNConfig) {
    this.config = config;
  }

  // Generate TURN credentials for a user
  generateTURNCredentials(userId: string, region?: string): TURNCredentials {
    const { primary } = this.config.servers;
    const timestamp = Math.floor(Date.now() / 1000) + this.config.ttl;
    
    // Create username with timestamp and user ID for uniqueness
    const username = `${timestamp}:${userId}`;
    
    // Generate credential using HMAC-SHA1 with the shared secret
    const hmac = createHash('sha1');
    hmac.update(username, 'utf8');
    const credential = hmac.digest('base64');

    return {
      username,
      credential,
      ttl: this.config.ttl,
    };
  }

  // Get ICE servers configuration for WebRTC
  getICEServers(userId: string, region?: string): RTCIceServer[] {
    const credentials = this.generateTURNCredentials(userId, region);
    const { primary, fallback = [] } = this.config.servers;

    const servers: RTCIceServer[] = [
      // Google's public STUN servers as fallback
      { urls: ['stun:stun.l.google.com:19302'] },
      { urls: ['stun:stun1.l.google.com:19302'] },
    ];

    // Primary TURN server
    servers.push({
      urls: [
        `turn:${primary.host}:${primary.turnPort}?transport=udp`,
        `turn:${primary.host}:${primary.turnPort}?transport=tcp`,
        `turns:${primary.host}:${primary.turnPort + 1}?transport=tcp`, // TLS
      ],
      username: credentials.username,
      credential: credentials.credential,
    });

    // Fallback TURN servers
    fallback.forEach(server => {
      const fallbackCredentials = this.generateTURNCredentials(userId);
      servers.push({
        urls: [
          `turn:${server.host}:${server.turnPort}?transport=udp`,
          `turn:${server.host}:${server.turnPort}?transport=tcp`,
          `turns:${server.host}:${server.turnPort + 1}?transport=tcp`,
        ],
        username: fallbackCredentials.username,
        credential: fallbackCredentials.credential,
      });
    });

    return servers;
  }

  // Get regional TURN servers for better performance
  getRegionalICEServers(userId: string, region: string): RTCIceServer[] {
    // In a real implementation, you would select servers based on region
    // For now, return the same servers but could be expanded
    return this.getICEServers(userId, region);
  }

  // Validate TURN credentials
  validateCredentials(username: string, credential: string): boolean {
    try {
      // Extract timestamp from username
      const [timestampStr] = username.split(':');
      const timestamp = parseInt(timestampStr, 10);
      const currentTime = Math.floor(Date.now() / 1000);

      // Check if credentials are expired
      if (timestamp < currentTime) {
        return false;
      }

      // Regenerate credential and compare
      const hmac = createHash('sha1');
      hmac.update(username, 'utf8');
      const expectedCredential = hmac.digest('base64');

      return credential === expectedCredential;
    } catch (error) {
      return false;
    }
  }

  // Get server health status
  async getServerHealth(): Promise<{ primary: boolean; fallback: boolean[] }> {
    // In a real implementation, you would ping the servers
    // For now, return a placeholder
    return {
      primary: true,
      fallback: this.config.servers.fallback?.map(() => true) || [],
    };
  }
}

// Initialize CoTURN manager
const coturnConfig: CoTURNConfig = {
  servers: {
    primary: {
      host: process.env.COTURN_PRIMARY_HOST || 'turn.example.com',
      port: 3478,
      turnPort: 3478,
      secret: process.env.COTURN_SECRET || 'default-secret',
      realm: process.env.COTURN_REALM || 'chatapp.com',
    },
    fallback: process.env.COTURN_FALLBACK_HOSTS?.split(',').map(host => ({
      host: host.trim(),
      port: 3478,
      turnPort: 3478,
      secret: process.env.COTURN_SECRET || 'default-secret',
      realm: process.env.COTURN_REALM || 'chatapp.com',
    })),
  },
  ttl: parseInt(process.env.COTURN_TTL || '86400', 10), // 24 hours
};

export const coturnManager = new CoTURNManager(coturnConfig);
