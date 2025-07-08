import { Server as SocketIOServer } from 'socket.io';
import { AuthenticatedSocket } from '../socket';
import { GroupRepository } from '../../database/repositories/group';
import { socketManager } from '../socket';

const groupRepository = new GroupRepository();

export function registerGroupEvents(socket: AuthenticatedSocket, io: SocketIOServer) {
  // Create group
  socket.on('group:create', async (data) => {
    try {
      const { name, description, participants, avatar } = data;

      // Create group
      const group = await groupRepository.createGroup({
        name,
        description,
        participants,
        createdBy: socket.userId as any,
        avatar,
      });

      // Join creator to group room
      socket.join(`chat:${group._id}`);

      // Notify all participants about new group
      participants.forEach((participantId: string) => {
        socketManager.emitToUser(participantId, 'group:created', {
          group,
          createdBy: {
            userId: socket.userId,
            displayName: socket.user.displayName,
          },
        });
      });

      socket.emit('group:create:success', { group });

    } catch (error) {
      console.error('Error creating group:', error);
      socket.emit('group:create:error', { message: 'Failed to create group' });
    }
  });

  // Join group room for real-time updates
  socket.on('group:join', async (data) => {
    try {
      const { groupId } = data;

      // Verify group membership
      const group = await groupRepository.findById(groupId);
      if (!group || !group.participants.includes(socket.userId as any)) {
        return socket.emit('error', { message: 'Not authorized to join this group' });
      }

      socket.join(`chat:${groupId}`);
      socket.emit('group:joined', { groupId });

    } catch (error) {
      console.error('Error joining group room:', error);
    }
  });

  // Add members to group
  socket.on('group:add-members', async (data) => {
    try {
      const { groupId, userIds } = data;

      // Check if user is admin or has permission
      const isAdmin = await groupRepository.isUserAdmin(groupId, socket.userId as any);
      if (!isAdmin) {
        const group = await groupRepository.findById(groupId);
        if (group?.groupInfo?.settings.whoCanAddMembers === 'admins') {
          return socket.emit('error', { message: 'Only admins can add members' });
        }
      }

      // Add members
      await groupRepository.addParticipants(groupId, userIds);

      // Get updated group
      const updatedGroup = await groupRepository.findById(groupId);

      // Notify existing group members
      io.to(`chat:${groupId}`).emit('group:members:added', {
        groupId,
        addedUsers: userIds,
        addedBy: socket.userId,
        group: updatedGroup,
      });

      // Notify new members
      userIds.forEach((userId: string) => {
        socketManager.emitToUser(userId, 'group:added', {
          group: updatedGroup,
          addedBy: {
            userId: socket.userId,
            displayName: socket.user.displayName,
          },
        });
      });

    } catch (error) {
      console.error('Error adding group members:', error);
      socket.emit('error', { message: 'Failed to add members' });
    }
  });

  // Remove member from group
  socket.on('group:remove-member', async (data) => {
    try {
      const { groupId, userId } = data;

      // Check if user is admin
      const isAdmin = await groupRepository.isUserAdmin(groupId, socket.userId as any);
      if (!isAdmin && socket.userId !== userId) {
        return socket.emit('error', { message: 'Only admins can remove members' });
      }

      // Remove member
      await groupRepository.removeParticipant(groupId, userId);

      // Notify group members
      io.to(`chat:${groupId}`).emit('group:member:removed', {
        groupId,
        removedUser: userId,
        removedBy: socket.userId,
      });

      // Notify removed user
      socketManager.emitToUser(userId, 'group:removed', {
        groupId,
        removedBy: {
          userId: socket.userId,
          displayName: socket.user.displayName,
        },
      });

    } catch (error) {
      console.error('Error removing group member:', error);
      socket.emit('error', { message: 'Failed to remove member' });
    }
  });

  // Promote user to admin
  socket.on('group:promote', async (data) => {
    try {
      const { groupId, userId } = data;

      // Check if user is admin
      const isAdmin = await groupRepository.isUserAdmin(groupId, socket.userId as any);
      if (!isAdmin) {
        return socket.emit('error', { message: 'Only admins can promote members' });
      }

      // Promote user
      await groupRepository.promoteToAdmin(groupId, userId);

      // Notify group members
      io.to(`chat:${groupId}`).emit('group:member:promoted', {
        groupId,
        promotedUser: userId,
        promotedBy: socket.userId,
      });

    } catch (error) {
      console.error('Error promoting group member:', error);
      socket.emit('error', { message: 'Failed to promote member' });
    }
  });

  // Demote admin
  socket.on('group:demote', async (data) => {
    try {
      const { groupId, userId } = data;

      // Check if user is admin
      const isAdmin = await groupRepository.isUserAdmin(groupId, socket.userId as any);
      if (!isAdmin) {
        return socket.emit('error', { message: 'Only admins can demote members' });
      }

      // Demote admin
      await groupRepository.demoteAdmin(groupId, userId);

      // Notify group members
      io.to(`chat:${groupId}`).emit('group:member:demoted', {
        groupId,
        demotedUser: userId,
        demotedBy: socket.userId,
      });

    } catch (error) {
      console.error('Error demoting group member:', error);
      socket.emit('error', { message: 'Failed to demote member' });
    }
  });

  // Leave group
  socket.on('group:leave', async (data) => {
    try {
      const { groupId } = data;

      // Leave group
      await groupRepository.leaveGroup(groupId, socket.userId as any);

      // Leave group room
      socket.leave(`chat:${groupId}`);

      // Notify remaining group members
      socket.to(`chat:${groupId}`).emit('group:member:left', {
        groupId,
        leftUser: socket.userId,
        user: {
          displayName: socket.user.displayName,
        },
      });

      socket.emit('group:left', { groupId });

    } catch (error) {
      console.error('Error leaving group:', error);
      socket.emit('error', { message: 'Failed to leave group' });
    }
  });

  // Update group info
  socket.on('group:update', async (data) => {
    try {
      const { groupId, name, description, avatar } = data;

      // Check permissions
      const group = await groupRepository.findById(groupId);
      const isAdmin = await groupRepository.isUserAdmin(groupId, socket.userId as any);
      
      if (group?.groupInfo?.settings.whoCanEditGroupInfo === 'admins' && !isAdmin) {
        return socket.emit('error', { message: 'Only admins can edit group info' });
      }

      // Update group
      const updatedGroup = await groupRepository.updateGroupInfo(groupId, {
        name,
        description,
        avatar,
      });

      // Notify group members
      io.to(`chat:${groupId}`).emit('group:updated', {
        group: updatedGroup,
        updatedBy: socket.userId,
      });

    } catch (error) {
      console.error('Error updating group:', error);
      socket.emit('error', { message: 'Failed to update group' });
    }
  });
}